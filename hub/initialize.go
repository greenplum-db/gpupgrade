package hub

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/db"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (s *Server) Initialize(in *idl.InitializeRequest, stream idl.CliToHub_InitializeServer) (err error) {
	st, err := BeginStep(s.StateDir, "initialize", stream)
	if err != nil {
		return err
	}

	defer func() {
		if ferr := st.Finish(); ferr != nil {
			err = multierror.Append(err, ferr).ErrorOrNil()
		}

		if err != nil {
			gplog.Error(fmt.Sprintf("initialize: %s", err))
		}
	}()

	st.Run(idl.Substep_CONFIG, func(stream step.OutStreams) error {
		return s.fillClusterConfigsSubStep(stream, in)
	})

	st.Run(idl.Substep_START_AGENTS, func(_ step.OutStreams) error {
		_, err := RestartAgents(context.Background(), nil, s.Source.GetHostnames(), s.AgentPort, s.StateDir)
		return err
	})

	return st.Err()
}

func (s *Server) InitializeCreateCluster(in *idl.InitializeCreateClusterRequest, stream idl.CliToHub_InitializeCreateClusterServer) (err error) {
	st, err := BeginStep(s.StateDir, "initialize", stream)
	if err != nil {
		return err
	}

	defer func() {
		if ferr := st.Finish(); ferr != nil {
			err = multierror.Append(err, ferr).ErrorOrNil()
		}

		if err != nil {
			gplog.Error(fmt.Sprintf("initialize: %s", err))
		}
	}()

	st.Run(idl.Substep_CREATE_TARGET_CONFIG, func(_ step.OutStreams) error {
		var err error
		targetMasterPort, err := s.GenerateInitsystemConfig(in.Ports)
		if err != nil {
			return err
		}

		// target master port is used for querying segment configuration.
		// once the target master port is decided, it's persisted in hub configuration
		// to allow further steps to use it in case they are being re-run after a failed
		// attempt.
		s.Config.TargetMasterPort = targetMasterPort
		if err := s.SaveConfig(); err != nil {
			return err
		}

		return err
	})

	st.Run(idl.Substep_INIT_TARGET_CLUSTER, func(stream step.OutStreams) error {
		return s.CreateTargetCluster(stream, s.Config.TargetMasterPort)
	})

	st.Run(idl.Substep_SHUTDOWN_TARGET_CLUSTER, func(stream step.OutStreams) error {
		return s.ShutdownCluster(stream, false)
	})

	st.Run(idl.Substep_BACKUP_TARGET_MASTER, func(stream step.OutStreams) error {
		sourceDir := s.Target.MasterDataDir()
		targetDir := filepath.Join(s.StateDir, originalMasterBackupName)
		return RsyncMasterDataDir(stream, sourceDir, targetDir)
	})

	st.AlwaysRun(idl.Substep_CHECK_UPGRADE, func(stream step.OutStreams) error {
		return s.CheckUpgrade(stream)
	})

	return st.Err()
}

// create old/new clusters, write to disk and re-read from disk to make sure it is "durable"
func (s *Server) fillClusterConfigsSubStep(_ step.OutStreams, request *idl.InitializeRequest) error {
	conn := db.NewDBConn("localhost", int(request.SourcePort), "template1")
	defer conn.Close()

	var err error
	s.Source, err = utils.ClusterFromDB(conn, request.SourceBinDir)
	if err != nil {
		return errors.Wrap(err, "could not retrieve source configuration")
	}

	s.Target = &utils.Cluster{BinDir: request.TargetBinDir}
	s.UseLinkMode = request.UseLinkMode

	if err := s.SaveConfig(); err != nil {
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
