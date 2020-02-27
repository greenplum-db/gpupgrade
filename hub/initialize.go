package hub

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/db"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (s *Server) Initialize(in *idl.InitializeRequest, stream idl.CliToHub_InitializeServer) (err error) {
	st, err := step.Begin(s.StateDir, "initialize", stream)
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
	st, err := step.Begin(s.StateDir, "initialize", stream)
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
		return s.GenerateInitsystemConfig()
	})

	st.Run(idl.Substep_INIT_TARGET_CLUSTER, func(stream step.OutStreams) error {
		return s.CreateTargetCluster(stream)
	})

	st.Run(idl.Substep_SHUTDOWN_TARGET_CLUSTER, func(stream step.OutStreams) error {
		return StopCluster(stream, s.Target, false)
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

	var ports []int
	for _, p := range request.Ports {
		ports = append(ports, int(p))
	}

	s.TargetPorts, err = assignPorts(s.Source, ports)
	if err != nil {
		return err
	}

	if err := s.SaveConfig(); err != nil {
		return err
	}

	return nil
}

func assignPorts(source *utils.Cluster, ports []int) (PortAssignments, error) {
	if len(ports) == 0 {
		return defaultTargetPorts(source), nil
	}

	ports = sanitize(ports)
	if err := checkTargetPorts(source, ports); err != nil {
		return PortAssignments{}, err
	}

	// Pop the first port off for master.
	masterPort := ports[0]
	ports = ports[1:]

	var standbyPort int
	if _, ok := source.Mirrors[-1]; ok {
		// Pop the next port off for standby.
		standbyPort = ports[0]
		ports = ports[1:]
	}

	return PortAssignments{
		Master:    masterPort,
		Standby:   standbyPort,
		Primaries: ports,
	}, nil
}

// sanitize sorts and deduplicates a slice of port numbers.
func sanitize(ports []int) []int {
	sort.Slice(ports, func(i, j int) bool { return ports[i] < ports[j] })

	dedupe := ports[:0] // point at the same backing array

	var last int
	for i, port := range ports {
		if i == 0 || port != last {
			dedupe = append(dedupe, port)
		}
		last = port
	}

	return dedupe
}

// defaultPorts generates the minimum temporary port range necessary to handle a
// cluster of the given topology. The first port in the list is meant to be used
// for the master.
func defaultTargetPorts(source *utils.Cluster) PortAssignments {
	// Partition segments by host in order to correctly assign ports.
	segmentsByHost := make(map[string][]utils.SegConfig)

	for content, segment := range source.Primaries {
		// Exclude the master for now. We want to give it its own reserved port,
		// which does not overlap with the other segments, so we'll add it back
		// later.
		if content == -1 {
			continue
		}
		segmentsByHost[segment.Hostname] = append(segmentsByHost[segment.Hostname], segment)
	}

	const masterPort = 50432
	nextPort := masterPort + 1

	var standbyPort int
	if _, ok := source.Mirrors[-1]; ok {
		// Reserve another port for the standby.
		standbyPort = nextPort
		nextPort++
	}

	// Reserve enough ports to handle the host with the most segments.
	var maxSegs int
	for _, segments := range segmentsByHost {
		if len(segments) > maxSegs {
			maxSegs = len(segments)
		}
	}

	var primaryPorts []int
	for i := 0; i < maxSegs; i++ {
		primaryPorts = append(primaryPorts, nextPort)
		nextPort++
	}

	return PortAssignments{
		Master:    masterPort,
		Standby:   standbyPort,
		Primaries: primaryPorts,
	}
}

// checkTargetPorts ensures that the temporary port range passed by the user has
// enough ports to cover a cluster of the given topology. This function assumes
// the port list has at least one port.
func checkTargetPorts(source *utils.Cluster, desiredPorts []int) error {
	if len(desiredPorts) == 0 {
		// failed precondition
		panic("checkTargetPorts() must be called with at least one port")
	}

	segmentsByHost := make(map[string][]utils.SegConfig)

	numAvailablePorts := len(desiredPorts)
	numAvailablePorts-- // master always takes one

	for content, segment := range source.Primaries {
		// Exclude the master; it's taken care of with the first port.
		if content == -1 {
			continue
		}
		segmentsByHost[segment.Hostname] = append(segmentsByHost[segment.Hostname], segment)
	}

	if _, ok := source.Mirrors[-1]; ok {
		// The standby will take a port from the pool.
		numAvailablePorts--
	}

	for _, segments := range segmentsByHost {
		if numAvailablePorts < len(segments) {
			return errors.New("not enough ports for each segment")
		}
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
