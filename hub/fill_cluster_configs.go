// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"database/sql"
	"os"
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

	config.Source = source
	config.TargetGPHome = request.TargetGPHome
	config.UseLinkMode = request.UseLinkMode

	var ports []int
	for _, p := range request.Ports {
		ports = append(ports, int(p))
	}

	config.TargetInitializeConfig, err = AssignDatadirsAndPorts(config.Source, ports, config.UpgradeID)
	if err != nil {
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

func getBinaryPath() (string, error) {
	hubPath, err := os.Executable()
	if err != nil {
		return "", err
	}

	return filepath.Join(filepath.Dir(hubPath), "gpupgrade"), nil
}
