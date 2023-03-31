// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"encoding/json"
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

func Create(hubPort int, agentPort int, sourceGPHome string, sourcePort int, targetGPHome string, mode idl.Mode, useHbaHostnames bool) (Config, error) {
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
