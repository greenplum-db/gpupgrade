// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/db"
	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

// FillConfiguration populates as much of the passed Config as possible, given a
// connection to the source cluster and the settings contained in an
// InitializeRequest from the client. The configuration is then saved to disk.
func FillConfiguration(config *Config, conn *sql.DB, _ step.OutStreams, request *idl.InitializeRequest, saveConfig func() error) error {
	config.AgentPort = int(request.AgentPort)
	config.UseHbaHostnames = request.UseHbaHostnames

	// Assign a new universal upgrade identifier.
	config.UpgradeID = upgrade.NewID()

	if err := CheckSourceClusterConfiguration(conn); err != nil {
		return err
	}

	// XXX ugly; we should just use the conn we're passed, but our DbConn
	// concept (which isn't really used) gets in the way
	dbconn := db.NewDBConn("localhost", int(request.SourcePort), "template1")
	source, err := greenplum.ClusterFromDB(dbconn, request.SourceGPHome)
	if err != nil {
		return xerrors.Errorf("retrieve source configuration: %w", err)
	}

	target := source // create target cluster based off source cluster
	config.Source = &source
	config.Target = &target
	config.TargetGPHome = request.TargetGPHome
	config.UseLinkMode = request.UseLinkMode

	var ports []int
	for _, p := range request.Ports {
		ports = append(ports, int(p))
	}

	config.IntermediateTarget, err = AssignDatadirsAndPorts(config.Source, ports, config.UpgradeID)
	if err != nil {
		return err
	}

	if err := ensureTempPortRangeDoesNotOverlapWithSourceClusterPorts(config.Source, config.IntermediateTarget); err != nil {
		return err
	}

	// major version upgrade requires upgrading tablespaces
	if dbconn.Version.Is("5") {
		if err := utils.System.MkdirAll(utils.GetTablespaceDir(), 0700); err != nil {
			return xerrors.Errorf("create tablespace directory %q: %w", utils.GetTablespaceDir(), err)
		}
		config.TablespacesMappingFilePath = filepath.Join(utils.GetTablespaceDir(), greenplum.TablespacesMappingFile)
		config.Tablespaces, err = greenplum.TablespacesFromDB(dbconn, config.TablespacesMappingFilePath)
		if err != nil {
			return xerrors.Errorf("extract tablespace information: %w", err)
		}
	}

	if err := saveConfig(); err != nil {
		return err
	}

	return nil
}

func AssignDatadirsAndPorts(source *greenplum.Cluster, ports []int, upgradeID upgrade.ID) (InitializeConfig, error) {
	ports = sanitize(ports)

	targetInitializeConfig := InitializeConfig{}

	var segPrefix string
	nextPortIndex := 0

	// XXX we can't handle a masterless cluster elsewhere in the code; we may
	// want to remove the "ok" check here and force NewCluster to error out
	if master, ok := source.Primaries[-1]; ok {
		// Reserve a port for the master.
		if nextPortIndex > len(ports)-1 {
			return InitializeConfig{}, errors.New("not enough ports")
		}

		// Save the segment prefix for later.
		var err error
		segPrefix, err = GetMasterSegPrefix(master.DataDir)
		if err != nil {
			return InitializeConfig{}, err
		}

		master.Port = ports[nextPortIndex]
		master.DataDir = upgrade.TempDataDir(master.DataDir, segPrefix, upgradeID)
		targetInitializeConfig.Master = master
		nextPortIndex++
	}

	if standby, ok := source.Mirrors[-1]; ok {
		// Reserve a port for the standby.
		if nextPortIndex > len(ports)-1 {
			return InitializeConfig{}, errors.New("not enough ports")
		}
		standby.Port = ports[nextPortIndex]
		standby.DataDir = upgrade.TempDataDir(standby.DataDir, segPrefix, upgradeID)
		targetInitializeConfig.Standby = standby
		nextPortIndex++
	}

	portIndexByHost := make(map[string]int)

	for _, content := range source.ContentIDs {
		// Skip the master segment
		if content == -1 {
			continue
		}

		segment := source.Primaries[content]

		if portIndex, ok := portIndexByHost[segment.Hostname]; ok {
			if portIndex > len(ports)-1 {
				return InitializeConfig{}, errors.New("not enough ports")
			}
			segment.Port = ports[portIndex]
			portIndexByHost[segment.Hostname]++
		} else {
			if nextPortIndex > len(ports)-1 {
				return InitializeConfig{}, errors.New("not enough ports")
			}
			segment.Port = ports[nextPortIndex]
			portIndexByHost[segment.Hostname] = nextPortIndex + 1
		}
		segment.DataDir = upgrade.TempDataDir(segment.DataDir, segPrefix, upgradeID)

		targetInitializeConfig.Primaries = append(targetInitializeConfig.Primaries, segment)
	}

	for _, content := range source.ContentIDs {
		// Skip the standby segment
		if content == -1 {
			continue
		}

		if segment, ok := source.Mirrors[content]; ok {
			if portIndex, ok := portIndexByHost[segment.Hostname]; ok {
				if portIndex > len(ports)-1 {
					return InitializeConfig{}, errors.New("not enough ports")
				}
				segment.Port = ports[portIndex]
				portIndexByHost[segment.Hostname]++
			} else {
				if nextPortIndex > len(ports)-1 {
					return InitializeConfig{}, errors.New("not enough ports")
				}
				segment.Port = ports[nextPortIndex]
				portIndexByHost[segment.Hostname] = nextPortIndex + 1
			}
			segment.DataDir = upgrade.TempDataDir(segment.DataDir, segPrefix, upgradeID)

			targetInitializeConfig.Mirrors = append(targetInitializeConfig.Mirrors, segment)
		}
	}

	return targetInitializeConfig, nil
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

func ensureTempPortRangeDoesNotOverlapWithSourceClusterPorts(source *greenplum.Cluster, target InitializeConfig) error {
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

	// check if temp target ports overlap with source cluster ports on a particular host
	targetPorts := []greenplum.SegConfig{target.Master, target.Standby}
	targetPorts = append(targetPorts, target.Primaries...)

	for _, seg := range targetPorts {
		if sourcePorts[HostPort{Host: seg.Hostname, Port: seg.Port}] {
			return newInvalidTempPortRangeError(seg.Hostname, seg.Port)
		}
	}
	for _, seg := range target.Mirrors {
		if sourcePorts[HostPort{Host: seg.Hostname, Port: seg.Port}] {
			return newInvalidTempPortRangeError(seg.Hostname, seg.Port)
		}
	}

	return nil
}

var ErrInvalidTempPortRange = errors.New("invalid temp_port range")

type InvalidTempPortRangeError struct {
	conflictingHost string
	conflictingPort int
}

func newInvalidTempPortRangeError(conflictingHost string, conflictingPort int) *InvalidTempPortRangeError {
	return &InvalidTempPortRangeError{conflictingHost: conflictingHost, conflictingPort: conflictingPort}
}

func (i *InvalidTempPortRangeError) Error() string {
	return fmt.Sprintf("temp_port_range contains port %d which overlaps with the source cluster ports on host %s. "+
		"Specify a non-overlapping temp_port_range.", i.conflictingPort, i.conflictingHost)
}

func (i *InvalidTempPortRangeError) Is(err error) bool {
	return err == ErrInvalidTempPortRange
}
