// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/config"
	"github.com/greenplum-db/gpupgrade/testutils"
)

func TestConfig(t *testing.T) {
	stateDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, stateDir)

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

	initialize_stopBeforeClusterCreation(t)
	defer revert(t)

	t.Run("configuration can be read piece by piece", func(t *testing.T) {
		actual := configShow(t, "--source-gphome")
		if actual != GPHOME_SOURCE {
			t.Errorf("got %q want %q", actual, GPHOME_SOURCE)
		}

		actual = configShow(t, "--target-gphome")
		if actual != GPHOME_TARGET {
			t.Errorf("got %q want %q", actual, GPHOME_TARGET)
		}
	})

	t.Run("configuration can be dumped as a whole", func(t *testing.T) {
		expected := []string{GPHOME_SOURCE, readConfig(t, `.Intermediate.Primaries."-1".DataDir`), GPHOME_TARGET, TARGET_PGPORT, readConfig(t, ".UpgradeID")}

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

func readConfig(t *testing.T, filter string) string {
	t.Helper()

	cmd := exec.Command("jq", "--raw-output", filter, config.GetConfigFile())
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %v stderr: %q", err, output)
	}

	return strings.TrimSpace(string(output))
}
