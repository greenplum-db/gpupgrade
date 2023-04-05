// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/config/backupdir"
	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

const ConfigFileName = "config.json"

type Config struct {
	// We do not combine the state directory and backup directory for
	// several reasons:
	// - The backup directory needs to be configurable since there
	// may not be enough space in the default location. If the state and
	// backup directories are combined and the backup directory needs to be
	// changed, then we have to preserve gpupgrade state by copying
	// substeps.json and config.json to the new location. This is awkward,
	// hard to manage, and error prone.
	// - The default state directory $HOME/.gpupgrade is known upfront with
	// no dependencies. Whereas the default backup directory is based on the
	// data directories. Having a state directory with no dependencies is
	// much easier to create and remove during the gpupgrade lifecycle.
	BackupDirs backupdir.BackupDirs

	// Source is the GPDB cluster that is being upgraded. It is populated during
	// the generation of the cluster config in the initialize step; before that,
	// it is nil.
	Source *greenplum.Cluster

	// Intermediate represents the initialized target cluster that is upgraded
	// based on the source.
	Intermediate *greenplum.Cluster

	// Target is the upgraded GPDB cluster. It is populated during the target
	// gpinitsystem execution in the initialize step; before that, it is nil.
	Target *greenplum.Cluster

	HubPort         int
	AgentPort       int
	Mode            idl.Mode
	UseHbaHostnames bool
	UpgradeID       upgrade.ID
}

func (conf *Config) Write() error {
	var buffer bytes.Buffer
	enc := json.NewEncoder(&buffer)
	enc.SetIndent("", "  ")
	if err := enc.Encode(conf); err != nil {
		return xerrors.Errorf("save configuration file: %w", err)
	}

	return utils.AtomicallyWrite(GetConfigFile(), buffer.Bytes())
}

func Read() (*Config, error) {
	contents, err := os.ReadFile(GetConfigFile())
	if err != nil {
		return nil, err
	}

	conf := &Config{}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	if err := decoder.Decode(conf); err != nil {
		return &Config{}, xerrors.Errorf("decode configuration file: %w", err)
	}

	return conf, nil
}

func GetConfigFile() string {
	return filepath.Join(utils.GetStateDir(), ConfigFileName)
}

func Create(db *sql.DB, hubPort int, agentPort int, sourceGPHome string, targetGPHome string, mode idl.Mode, useHbaHostnames bool, ports []int, parentBackupDirs string) (Config, error) {
	source, err := greenplum.ClusterFromDB(db, sourceGPHome, idl.ClusterDestination_source)
	if err != nil {
		return Config{}, xerrors.Errorf("retrieve source configuration: %w", err)
	}

	// Ensure segments are up, synchronized, and in their preferred role before proceeding.
	err = greenplum.WaitForSegments(db, 5*time.Minute, &source)
	if err != nil {
		return Config{}, err
	}

	targetVersion, err := greenplum.Version(targetGPHome)
	if err != nil {
		return Config{}, err
	}

	config := Config{}
	config.HubPort = hubPort
	config.AgentPort = agentPort
	config.Mode = mode
	config.UseHbaHostnames = useHbaHostnames
	config.UpgradeID = upgrade.NewID()
	config.BackupDirs, err = backupdir.ParseParentBackupDirs(parentBackupDirs, source)
	if err != nil {
		return Config{}, err
	}

	target := source // create target cluster based off source cluster
	config.Source = &source

	config.Target = &target
	config.Target.Destination = idl.ClusterDestination_target
	config.Target.GPHome = targetGPHome
	config.Target.Version = targetVersion

	config.Intermediate, err = GenerateIntermediateCluster(config.Source, ports, config.UpgradeID, config.Target.Version, config.Target.GPHome)
	if err != nil {
		return Config{}, err
	}

	if err := EnsureTempPortRangeDoesNotOverlapWithSourceClusterPorts(config.Source, config.Intermediate); err != nil {
		return Config{}, err
	}

	if config.Source.Version.Major == 5 {
		config.Source.Tablespaces, err = greenplum.TablespacesFromDB(db, utils.GetStateDirOldTablespacesFile())
		if err != nil {
			return Config{}, xerrors.Errorf("extract tablespace information: %w", err)
		}
	}

	return config, nil
}
