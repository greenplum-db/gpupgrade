package hub

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/db"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (h *Hub) Initialize(in *idl.InitializeRequest, stream idl.CliToHub_InitializeServer) (err error) {
	s, err := BeginStep(h.StateDir, "initialize", stream)
	if err != nil {
		return err
	}

	defer func() {
		if ferr := s.Finish(); ferr != nil {
			err = multierror.Append(err, ferr).ErrorOrNil()
		}

		if err != nil {
			gplog.Error(fmt.Sprintf("initialize: %s", err))
		}
	}()

	s.Run(idl.Substep_CONFIG, func(stream step.OutStreams) error {
		return h.fillClusterConfigsSubStep(stream, in)
	})

	s.Run(idl.Substep_START_AGENTS, func(_ step.OutStreams) error {
		_, err := RestartAgents(context.Background(), nil, h.Source.GetHostnames(), h.AgentPort, h.StateDir)
		return err
	})

	return s.Err()
}

func (h *Hub) InitializeCreateCluster(in *idl.InitializeCreateClusterRequest, stream idl.CliToHub_InitializeCreateClusterServer) (err error) {
	s, err := BeginStep(h.StateDir, "initialize", stream)
	if err != nil {
		return err
	}

	defer func() {
		if ferr := s.Finish(); ferr != nil {
			err = multierror.Append(err, ferr).ErrorOrNil()
		}

		if err != nil {
			gplog.Error(fmt.Sprintf("initialize: %s", err))
		}
	}()

	s.Run(idl.Substep_CREATE_TARGET_CONFIG, func(_ step.OutStreams) error {
		var err error
		targetMasterPort, err := h.GenerateInitsystemConfig(in.Ports)
		if err != nil {
			return err
		}

		// target master port is used for querying segment configuration.
		// once the target master port is decided, it's persisted in hub configuration
		// to allow further steps to use it in case they are being re-run after a failed
		// attempt.
		h.Config.TargetMasterPort = targetMasterPort
		if err := h.SaveConfig(); err != nil {
			return err
		}

		return err
	})

	s.Run(idl.Substep_SHUTDOWN_SOURCE_CLUSTER, func(stream step.OutStreams) error {
		return StopCluster(stream, h.Source)
	})

	s.Run(idl.Substep_INIT_TARGET_CLUSTER, func(stream step.OutStreams) error {
		return h.CreateTargetCluster(stream, h.Config.TargetMasterPort)
	})

	s.Run(idl.Substep_SHUTDOWN_TARGET_CLUSTER, func(stream step.OutStreams) error {
		return h.ShutdownCluster(stream, false)
	})

	s.AlwaysRun(idl.Substep_CHECK_UPGRADE, func(stream step.OutStreams) error {
		return h.CheckUpgrade(stream)
	})

	return s.Err()
}

// create old/new clusters, write to disk and re-read from disk to make sure it is "durable"
func (h *Hub) fillClusterConfigsSubStep(_ step.OutStreams, request *idl.InitializeRequest) error {
	conn := db.NewDBConn("localhost", int(request.SourcePort), "template1")
	defer conn.Close()

	var err error
	h.Source, err = utils.ClusterFromDB(conn, request.SourceBinDir)
	if err != nil {
		return errors.Wrap(err, "could not retrieve source configuration")
	}

	h.Target = &utils.Cluster{Cluster: new(cluster.Cluster), BinDir: request.TargetBinDir}
	h.UseLinkMode = request.UseLinkMode

	if err := h.SaveConfig(); err != nil {
		return err
	}

	return nil
}

func getAgentPath() (string, error) {
	hubPath, err := os.Executable()
	if err != nil {
		return "", err
	}

	return filepath.Join(filepath.Dir(hubPath), "gpupgrade"), nil
}
