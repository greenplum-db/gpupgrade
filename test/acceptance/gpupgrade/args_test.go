// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/greenplum-db/gpupgrade/cli/commands"
	"github.com/greenplum-db/gpupgrade/testutils"
)

func TestArgs(t *testing.T) {
	stateDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, stateDir)

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

	killServices(t)

	t.Run("gpupgrade initialize fails when passed insufficient arguments", func(t *testing.T) {
		cmd := exec.Command("gpupgrade", "initialize",
			"--non-interactive",
		)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("expected error got nil")
		}

		expected := commands.InitializeHelp + "\n"
		if string(output) != expected {
			t.Errorf("got %q want %q", string(output), expected)
		}
	})

	t.Run("gpupgrade initialize fails when other flags are used with --file", func(t *testing.T) {
		cmd := exec.Command("gpupgrade", "initialize",
			"--non-interactive",
			"--file", "/some/config",
			"--source-gphome", "/usr/local/gpdb5",
		)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("expected error got nil")
		}

		expected := "Error: The file flag cannot be used with any other flag except verbose and non-interactive.\n"
		if string(output) != expected {
			t.Errorf("got %q want %q", string(output), expected)
		}
	})

	t.Run("gpupgrade initialize fails when --pg-upgrade-verbose is used without --verbose", func(t *testing.T) {
		cmd := exec.Command("gpupgrade", "initialize",
			"--non-interactive",
			"--source-gphome", GPHOME_SOURCE,
			"--target-gphome", GPHOME_TARGET,
			"--source-master-port", PGPORT,
			"--stop-before-cluster-creation",
			"--disk-free-ratio", "0",
			"--pg-upgrade-verbose",
		)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("expected error got nil")
		}

		expected := "Error: expected --verbose when using --pg-upgrade-verbose\n"
		if string(output) != expected {
			t.Errorf("got %q want %q", string(output), expected)
		}
	})

	t.Run("gpupgrade initialize --file with verbose uses the configured values", func(t *testing.T) {
		configFile := filepath.Join(stateDir, "gpupgrade_config")
		contents := fmt.Sprintf(`source-gphome = %s
target-gphome = %s
source-master-port = %s
disk-free-ratio = 0
stop-before-cluster-creation = true
`, GPHOME_SOURCE, GPHOME_TARGET, PGPORT)
		testutils.MustWriteToFile(t, configFile, contents)

		cmd := exec.Command("gpupgrade", "initialize",
			"--non-interactive",
			"--verbose",
			"--pg-upgrade-verbose",
			"--file", configFile,
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("unexpected err: %v stderr: %q", err, output)
		}

		actual := configShow(t, "--source-gphome")
		if actual != GPHOME_SOURCE {
			t.Errorf("got %q want %q", actual, GPHOME_SOURCE)
		}

		actual = configShow(t, "--target-gphome")
		if actual != GPHOME_TARGET {
			t.Errorf("got %q want %q", actual, GPHOME_TARGET)
		}
	})

	t.Run("initialize sanitizes source-gphome and target-gphome", func(t *testing.T) {
		cmd := exec.Command("gpupgrade", "initialize",
			"--non-interactive",
			"--source-gphome", GPHOME_SOURCE+string(os.PathSeparator),
			"--target-gphome", GPHOME_TARGET+string(os.PathSeparator)+string(os.PathSeparator),
			"--source-master-port", PGPORT,
			"--stop-before-cluster-creation",
			"--disk-free-ratio", "0",
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("unexpected err: %v stderr: %q", err, output)
		}

		actual := configShow(t, "--source-gphome")
		if actual != GPHOME_SOURCE {
			t.Errorf("got %q want %q", actual, GPHOME_SOURCE)
		}

		actual = configShow(t, "--target-gphome")
		if actual != GPHOME_TARGET {
			t.Errorf("got %q want %q", actual, GPHOME_TARGET)
		}
	})

	t.Run("gpupgrade execute fails when --pg-upgrade-verbose is used without --verbose", func(t *testing.T) {
		initialize(t)

		cmd := exec.Command("gpupgrade", "execute",
			"--non-interactive",
			"--pg-upgrade-verbose",
		)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("expected error got nil")
		}

		expected := "Error: expected --verbose when using --pg-upgrade-verbose\n"
		if string(output) != expected {
			t.Errorf("got %q want %q", string(output), expected)
		}
	})
}
