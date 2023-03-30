// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

var InitializeConnectionFunc = initializeConnection

func initializeConnection(gphome string, port int) (*sql.DB, error) {
	tempSource, err := greenplum.NewCluster([]greenplum.SegConfig{})
	if err != nil {
		return nil, err
	}

	tempSource.Version, err = greenplum.Version(gphome)
	if err != nil {
		return nil, err
	}

	tempSource.Destination = idl.ClusterDestination_source
	conn := tempSource.Connection([]greenplum.Option{greenplum.Port(port)}...)
	db, err := sql.Open("pgx", conn)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func GetInitializeConfiguration(hubPort int, request *idl.InitializeRequest, wasEarlyExit bool) (*Config, error) {
	db, err := InitializeConnectionFunc(request.GetSourceGPHome(), int(request.GetSourcePort()))
	defer func() {
		if cErr := db.Close(); cErr != nil {
			err = errorlist.Append(err, cErr)
		}
	}()

	source, err := greenplum.ClusterFromDB(db, request.GetSourceGPHome(), idl.ClusterDestination_source)
	if err != nil {
		return nil, xerrors.Errorf("retrieve source configuration: %w", err)
	}

	config := &Config{}
	config.Source = &source
	config.UpgradeID = upgrade.NewID()
	config.HubPort = hubPort

	// We only need specific config values to be set for the hub RevertResponse
	// to handle reverting an early Initialize exit.
	if wasEarlyExit {
		return config, nil
	}

	err = greenplum.WaitForSegments(db, 5*time.Minute, &source)
	if err != nil {
		return nil, err
	}

	target := source // create target cluster based off source cluster
	targetVersion, err := greenplum.Version(request.GetTargetGPHome())
	if err != nil {
		return nil, err
	}

	config.AgentPort = int(request.GetAgentPort())
	config.UseHbaHostnames = request.GetUseHbaHostnames()
	config.Target = &target
	config.Target.Destination = idl.ClusterDestination_target
	config.Target.GPHome = request.GetTargetGPHome()
	config.Target.Version = targetVersion
	config.Mode = request.GetMode()

	var ports []int
	for _, p := range request.GetPorts() {
		ports = append(ports, int(p))
	}

	config.Intermediate, err = GenerateIntermediateCluster(config.Source, ports, config.UpgradeID, config.Target.Version, request.GetTargetGPHome())
	if err != nil {
		return nil, err
	}

	if err := EnsureTempPortRangeDoesNotOverlapWithSourceClusterPorts(config.Source, config.Intermediate); err != nil {
		return nil, err
	}

	if config.Source.Version.Major == 5 {
		config.Source.Tablespaces, err = greenplum.TablespacesFromDB(db, utils.GetStateDirOldTablespacesFile())
		if err != nil {
			return nil, xerrors.Errorf("extract tablespace information: %w", err)
		}
	}

	return config, nil
}

func GenerateIntermediateCluster(source *greenplum.Cluster, ports []int, upgradeID upgrade.ID, version semver.Version, gphome string) (*greenplum.Cluster, error) {
	ports = utils.Sanitize(ports)

	intermediate, err := greenplum.NewCluster([]greenplum.SegConfig{})
	if err != nil {
		return &greenplum.Cluster{}, err
	}

	var segPrefix string
	nextPortIndex := 0

	// XXX we can't handle a coordinatorless cluster elsewhere in the code; we may
	// want to remove the "ok" check here and force NewCluster to error out
	if coordinator, ok := source.Primaries[-1]; ok {
		// Reserve a port for the coordinator.
		if nextPortIndex > len(ports)-1 {
			return &greenplum.Cluster{}, errors.New("not enough ports")
		}

		// Save the segment prefix for later.
		var err error
		segPrefix, err = greenplum.GetCoordinatorSegPrefix(coordinator.DataDir)
		if err != nil {
			return &greenplum.Cluster{}, err
		}

		coordinator.Port = ports[nextPortIndex]
		coordinator.DataDir = upgrade.TempDataDir(coordinator.DataDir, segPrefix, upgradeID)
		intermediate.Primaries[-1] = coordinator
		nextPortIndex++
	}

	if standby, ok := source.Mirrors[-1]; ok {
		// Reserve a port for the standby.
		if nextPortIndex > len(ports)-1 {
			return &greenplum.Cluster{}, errors.New("not enough ports")
		}
		standby.Port = ports[nextPortIndex]
		standby.DataDir = upgrade.TempDataDir(standby.DataDir, segPrefix, upgradeID)
		intermediate.Mirrors[-1] = standby
		nextPortIndex++
	}

	portIndexByHost := make(map[string]int)

	var contents []int
	for content := range source.Primaries {
		contents = append(contents, content)
	}

	for content := range source.Mirrors {
		contents = append(contents, content)
	}

	contents = utils.Sanitize(contents)

	for _, content := range contents {
		if content == -1 {
			continue
		}

		segment := source.Primaries[content]

		if portIndex, ok := portIndexByHost[segment.Hostname]; ok {
			if portIndex > len(ports)-1 {
				return &greenplum.Cluster{}, errors.New("not enough ports")
			}
			segment.Port = ports[portIndex]
			portIndexByHost[segment.Hostname]++
		} else {
			if nextPortIndex > len(ports)-1 {
				return &greenplum.Cluster{}, errors.New("not enough ports")
			}
			segment.Port = ports[nextPortIndex]
			portIndexByHost[segment.Hostname] = nextPortIndex + 1
		}
		segment.DataDir = upgrade.TempDataDir(segment.DataDir, segPrefix, upgradeID)

		intermediate.Primaries[content] = segment
	}

	for _, content := range contents {
		if content == -1 {
			continue
		}

		if segment, ok := source.Mirrors[content]; ok {
			if portIndex, ok := portIndexByHost[segment.Hostname]; ok {
				if portIndex > len(ports)-1 {
					return &greenplum.Cluster{}, errors.New("not enough ports")
				}
				segment.Port = ports[portIndex]
				portIndexByHost[segment.Hostname]++
			} else {
				if nextPortIndex > len(ports)-1 {
					return &greenplum.Cluster{}, errors.New("not enough ports")
				}
				segment.Port = ports[nextPortIndex]
				portIndexByHost[segment.Hostname] = nextPortIndex + 1
			}
			segment.DataDir = upgrade.TempDataDir(segment.DataDir, segPrefix, upgradeID)

			intermediate.Mirrors[content] = segment
		}
	}

	intermediate.GPHome = gphome
	intermediate.Version = version
	intermediate.Destination = idl.ClusterDestination_intermediate

	return &intermediate, nil
}

func EnsureTempPortRangeDoesNotOverlapWithSourceClusterPorts(source *greenplum.Cluster, intermediate *greenplum.Cluster) error {
	type HostPort struct {
		Host string
		Port int
	}

	// create a set of source cluster HostPort's
	sourcePorts := make(map[HostPort]bool)
	for _, seg := range source.Primaries {
		sourcePorts[HostPort{Host: seg.Hostname, Port: seg.Port}] = true
	}
	for _, seg := range source.Mirrors {
		sourcePorts[HostPort{Host: seg.Hostname, Port: seg.Port}] = true
	}

	// check if intermediate target cluster ports overlap with source cluster ports on a particular host
	for _, seg := range intermediate.Primaries {
		if sourcePorts[HostPort{Host: seg.Hostname, Port: seg.Port}] {
			return newInvalidTempPortRangeError(seg.Hostname, seg.Port)
		}
	}
	for _, seg := range intermediate.Mirrors {
		if sourcePorts[HostPort{Host: seg.Hostname, Port: seg.Port}] {
			return newInvalidTempPortRangeError(seg.Hostname, seg.Port)
		}
	}

	return nil
}

var ErrInvalidTempPortRange = errors.New("invalid temp_port range")

type InvalidTempPortRangeError struct {
	ConflictingHost string
	ConflictingPort int
}

func newInvalidTempPortRangeError(conflictingHost string, conflictingPort int) *InvalidTempPortRangeError {
	return &InvalidTempPortRangeError{ConflictingHost: conflictingHost, ConflictingPort: conflictingPort}
}

func (i *InvalidTempPortRangeError) Error() string {
	return fmt.Sprintf("temp_port_range contains port %d which overlaps with the source cluster ports on host %s. "+
		"Specify a non-overlapping temp_port_range.", i.ConflictingPort, i.ConflictingHost)
}

func (i *InvalidTempPortRangeError) Is(err error) bool {
	return err == ErrInvalidTempPortRange
}