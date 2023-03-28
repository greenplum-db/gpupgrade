// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"os"
	"reflect"
	"testing"

	"github.com/greenplum-db/gpupgrade/config"
	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
)

func TestConfig(t *testing.T) {
	source, target := testutils.CreateMultinodeSampleClusterPair("/tmp")
	conf := &config.Config{
		Source:       source,
		Target:       target,
		Intermediate: &greenplum.Cluster{},
		Port:         12345,
		AgentPort:    54321,
		Mode:         idl.Mode_copy,
		UpgradeID:    0,
	}

	t.Run("save configuration contents to disk and load it back", func(t *testing.T) {
		stateDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, stateDir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
		defer resetEnv()

		// Write the configuration file.
		if err := conf.Save(); err != nil {
			t.Errorf("saving config: %+v", err)
		}

		// Reload the configuration and ensure the contents are the same.
		actual := new(config.Config)
		if err := actual.Load(); err != nil {
			t.Errorf("loading config: %+v", err)
		}

		if !reflect.DeepEqual(actual, conf) {
			t.Errorf("wrote config %#v but wanted %#v", actual, conf)
		}
	})
}

func TestConfigFileCreation(t *testing.T) {
	const hubPort = -1
	const sourcePort = 8888
	const sourceGPHome = "/mock/gphome"
	var sourceOld os.FileInfo

	stateDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, stateDir)

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

	t.Run("test idempotence", func(t *testing.T) {

		{ // creates initial cluster config files if none exist or fails"
			err := config.Create(hubPort, sourcePort, sourceGPHome)
			if err != nil {
				t.Fatalf("unexpected error %#v", err)
			}

			if sourceOld, err = os.Stat(config.GetConfigFile()); err != nil {
				t.Errorf("unexpected error %#v", err)
			}
		}

		{ // creating cluster config files is idempotent
			err := config.Create(hubPort, sourcePort, sourceGPHome)
			if err != nil {
				t.Fatalf("unexpected error %#v", err)
			}

			var sourceNew os.FileInfo
			if sourceNew, err = os.Stat(config.GetConfigFile()); err != nil {
				t.Errorf("got unexpected error %#v", err)
			}

			if sourceOld.ModTime() != sourceNew.ModTime() {
				t.Errorf("want %#v got %#v", sourceOld.ModTime(), sourceNew.ModTime())
			}
		}

		{ // creating cluster config files succeeds on multiple runs
			err := config.Create(hubPort, sourcePort, sourceGPHome)
			if err != nil {
				t.Fatalf("unexpected error %#v", err)
			}
		}
	})
}
