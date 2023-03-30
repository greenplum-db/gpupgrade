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
		HubPort:      12345,
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

func TestCreate(t *testing.T) {
	const hubPort = 9999
	const agentPort = 8888
	const sourceGPHome = "/mock/source-gphome"
	const sourcePort = 1234
	const targetGPHome = "/mock/target-gphome"
	const mode = idl.Mode_link
	const useHbaHostnames = false

	t.Run("test idempotence", func(t *testing.T) {
		stateDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, stateDir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
		defer resetEnv()

		var sourceOld os.FileInfo

		{ // creates initial cluster config files if none exist or fails"
			conf, err := config.Create(hubPort, agentPort, sourceGPHome, sourcePort, targetGPHome, mode, useHbaHostnames)
			if err != nil {
				t.Fatalf("unexpected error %#v", err)
			}

			err = conf.Save()
			if err != nil {
				t.Errorf("unexpected error %#v", err)
			}

			sourceOld, err = os.Stat(config.GetConfigFile())
			if err != nil {
				t.Errorf("unexpected error %#v", err)
			}
		}

		{ // creating cluster config files is idempotent
			_, err := config.Create(hubPort, agentPort, sourceGPHome, sourcePort, targetGPHome, mode, useHbaHostnames)
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
			_, err := config.Create(hubPort, agentPort, sourceGPHome, sourcePort, targetGPHome, mode, useHbaHostnames)
			if err != nil {
				t.Fatalf("unexpected error %#v", err)
			}
		}
	})

	t.Run("create adds known parameters", func(t *testing.T) {
		stateDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, stateDir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
		defer resetEnv()

		conf, err := config.Create(hubPort, agentPort, sourceGPHome, sourcePort, targetGPHome, mode, useHbaHostnames)
		if err != nil {
			t.Fatalf("unexpected error %#v", err)
		}

		err = conf.Save()
		if err != nil {
			t.Fatalf("unexpected error %#v", err)
		}

		if conf.HubPort != hubPort {
			t.Fatalf("got %d want %d", conf.HubPort, hubPort)
		}

		if conf.AgentPort != agentPort {
			t.Fatalf("got %d want %d", conf.AgentPort, agentPort)
		}

		if conf.Source.GPHome != sourceGPHome {
			t.Fatalf("got %s want %s", conf.Source.GPHome, sourceGPHome)
		}

		if conf.Source.CoordinatorPort() != sourcePort {
			t.Fatalf("got %d want %d", conf.Source.CoordinatorPort(), sourcePort)
		}

		if conf.Target.GPHome != targetGPHome {
			t.Fatalf("got %s want %s", conf.Target.GPHome, targetGPHome)
		}

		if conf.Mode != mode {
			t.Fatalf("got %s want %s", conf.Mode, mode)
		}

		if conf.UseHbaHostnames != useHbaHostnames {
			t.Fatalf("got %t want %t", conf.UseHbaHostnames, useHbaHostnames)
		}
	})

	t.Run("create adds upgradeID", func(t *testing.T) {
		stateDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, stateDir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
		defer resetEnv()
		
		conf, err := config.Create(hubPort, agentPort, sourceGPHome, sourcePort, targetGPHome, mode, useHbaHostnames)
		if err != nil {
			t.Fatalf("unexpected error %#v", err)
		}

		err = conf.Save()
		if err != nil {
			t.Fatalf("unexpected error %#v", err)
		}

		if conf.UpgradeID == 0 {
			t.Fatalf("expected non-empty UpgradeID")
		}
	})
}
