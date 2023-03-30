// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/config/backupdir"
	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

const ConfigFileName = "config.json"

// Config contains all the information that will be persisted to/loaded from
// from disk during calls to Save() and Load().
type Config struct {
	LogArchiveDir string

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

func (c *Config) Load() error {
	path := GetConfigFile()
	file, err := os.Open(path)
	if err != nil {
		return xerrors.Errorf("opening configuration file: %w", err)
	}
	defer file.Close()

	dec := json.NewDecoder(file)
	if err := dec.Decode(c); err != nil {
		return xerrors.Errorf("load configuration file: %w", err)
	}

	return nil
}

// SaveConfig persists the hub's configuration to disk.
func (c *Config) Save() error {
	var buffer bytes.Buffer
	enc := json.NewEncoder(&buffer)
	enc.SetIndent("", "  ")
	if err := enc.Encode(c); err != nil {
		return xerrors.Errorf("save configuration file: %w", err)
	}

	return utils.AtomicallyWrite(GetConfigFile(), buffer.Bytes())
}

func GetConfigFile() string {
	return filepath.Join(utils.GetStateDir(), ConfigFileName)
}

func Create(hubPort int, agentPort int, sourceGPHome string, sourcePort int, targetGPHome string, mode idl.Mode, useHbaHostnames bool) (Config, error) {
	path := GetConfigFile()
	exist, err := upgrade.PathExist(path)
	if err != nil {
		return Config{}, xerrors.Errorf("checking configuration path %q: %w", path, err)
	}

	if exist {
		log.Printf("Configuration file %s already present. Skipping.", path)
		return Config{}, err
	}

	// Bootstrap with known values early on so helper functions can be used.
	// For example, bootstrap with the hub port such that connecting to the hub
	// succeeds. Bootstrap with the source and target cluster GPHOME's, and
	// source cluster port such that when initialize exits early, revert has
	// enough information to succeed.
	config := Config{}
	config.HubPort = hubPort
	config.AgentPort = agentPort
	config.Mode = mode
	config.UseHbaHostnames = useHbaHostnames
	config.UpgradeID = upgrade.NewID()

	config.Source = &greenplum.Cluster{}
	config.Source.Primaries = make(greenplum.ContentToSegConfig)
	config.Source.Primaries[-1] = greenplum.SegConfig{Port: sourcePort}
	config.Source.GPHome = sourceGPHome

	config.Target = &greenplum.Cluster{}
	config.Target.GPHome = targetGPHome

	return config, nil
}
