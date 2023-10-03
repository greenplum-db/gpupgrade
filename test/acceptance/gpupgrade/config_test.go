// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/config"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/acceptance"
)

func TestConfig(t *testing.T) {
	stateDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, stateDir)

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

	acceptance.Initialize_stopBeforeClusterCreation(t)
	defer acceptance.Revert(t)

	t.Run("configuration can be read piece by piece", func(t *testing.T) {
		actual := configShow(t, "--source-gphome")
		if actual != acceptance.GPHOME_SOURCE {
			t.Errorf("got %q want %q", actual, acceptance.GPHOME_SOURCE)
		}

		actual = configShow(t, "--target-gphome")
		if actual != acceptance.GPHOME_TARGET {
			t.Errorf("got %q want %q", actual, acceptance.GPHOME_TARGET)
		}
	})

	t.Run("configuration can be dumped as a whole", func(t *testing.T) {
		expected := []string{
			acceptance.GPHOME_SOURCE,
			acceptance.Jq(t, config.GetConfigFile(), `.Intermediate.Primaries."-1".DataDir`),
			acceptance.GPHOME_TARGET,
			acceptance.TARGET_PGPORT,
			acceptance.Jq(t, config.GetConfigFile(), ".UpgradeID"),
		}

		output := configShow(t, "")
		lines := strings.Split(strings.TrimSpace(output), "\n")
		for i, line := range lines {
			actual := strings.SplitN(line, ": ", 2)[1]
			if actual != expected[i] {
				t.Errorf("got %q want %q", actual, expected[i])
			}
		}
	})
}

func configShow(t *testing.T, parameter string) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "config", "show", parameter)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %v stderr: %q", err, output)
	}

	return strings.TrimSpace(string(output))
}
