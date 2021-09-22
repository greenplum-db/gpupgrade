// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

// FillConfiguration populates the Config saves it to disk.
func FillConfiguration(config *Config, request *idl.InitializeRequest, conn *greenplum.Conn, saveConfig func() error) error {
	options := []greenplum.Option{
		greenplum.ToSource(),
		greenplum.Port(int(request.GetSourcePort())),
		greenplum.UtilityMode(),
	}

	db, err := sql.Open("pgx", conn.URI(options...))
	if err != nil {
		return err
	}
	defer func() {
		if cErr := db.Close(); cErr != nil {
			err = errorlist.Append(err, cErr)
		}
	}()

	config.AgentPort = int(request.GetAgentPort())
	config.UseHbaHostnames = request.GetUseHbaHostnames()
	config.UpgradeID = upgrade.NewID()

	source, err := greenplum.ClusterFromDB(db, conn.SourceVersion, request.GetSourceGPHome(), idl.ClusterDestination_SOURCE)
	if err != nil {
		return xerrors.Errorf("retrieve source configuration: %w", err)
	}

	err = source.WaitForClusterToBeReady(conn)
	if err != nil {
		return err
	}

	target := source // create target cluster based off source cluster
	config.Source = &source
	config.Target = &target
	config.Target.Destination = idl.ClusterDestination_TARGET
	config.Target.GPHome = request.GetTargetGPHome()
	config.Target.Version = conn.TargetVersion
	config.UseLinkMode = request.GetUseLinkMode()

	var ports []int
	for _, p := range request.GetPorts() {
		ports = append(ports, int(p))
	}

	config.IntermediateTarget, err = GenerateIntermediateTargetCluster(config.Source, ports, config.UpgradeID, conn.TargetVersion, request.GetTargetGPHome())
	if err != nil {
		return err
	}

	if err := ensureTempPortRangeDoesNotOverlapWithSourceClusterPorts(config.Source, config.IntermediateTarget); err != nil {
		return err
	}

	if config.Source.Version.Major == 5 {
		if err := utils.System.MkdirAll(utils.GetTablespaceDir(), 0700); err != nil {
			return xerrors.Errorf("create tablespace directory %q: %w", utils.GetTablespaceDir(), err)
		}

		config.TablespacesMappingFilePath = filepath.Join(utils.GetTablespaceDir(), greenplum.TablespacesMappingFile)
		config.Tablespaces, err = greenplum.TablespacesFromDB(db, config.TablespacesMappingFilePath)
		if err != nil {
			return xerrors.Errorf("extract tablespace information: %w", err)
		}
	}

	if err := saveConfig(); err != nil {
		return err
	}

	return nil
}

func GenerateIntermediateTargetCluster(source *greenplum.Cluster, ports []int, upgradeID upgrade.ID, version semver.Version, gphome string) (*greenplum.Cluster, error) {
	ports = utils.Sanitize(ports)

	intermediate, err := greenplum.NewCluster([]greenplum.SegConfig{})
	if err != nil {
		return &greenplum.Cluster{}, err
	}

	var segPrefix string
	nextPortIndex := 0

	// XXX we can't handle a masterless cluster elsewhere in the code; we may
	// want to remove the "ok" check here and force NewCluster to error out
	if master, ok := source.Primaries[-1]; ok {
		// Reserve a port for the master.
		if nextPortIndex > len(ports)-1 {
			return &greenplum.Cluster{}, errors.New("not enough ports")
		}

		// Save the segment prefix for later.
		var err error
		segPrefix, err = GetMasterSegPrefix(master.DataDir)
		if err != nil {
			return &greenplum.Cluster{}, err
		}

		master.Port = ports[nextPortIndex]
		master.DataDir = upgrade.TempDataDir(master.DataDir, segPrefix, upgradeID)
		intermediate.Primaries[-1] = master
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
	intermediate.Destination = idl.ClusterDestination_INTERMEDIATE

	return &intermediate, nil
}

func ensureTempPortRangeDoesNotOverlapWithSourceClusterPorts(source *greenplum.Cluster, intermediateTarget *greenplum.Cluster) error {
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
	for _, seg := range intermediateTarget.Primaries {
		if sourcePorts[HostPort{Host: seg.Hostname, Port: seg.Port}] {
			return newInvalidTempPortRangeError(seg.Hostname, seg.Port)
		}
	}
	for _, seg := range intermediateTarget.Mirrors {
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
