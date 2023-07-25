// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"errors"
	"os"
	"os/exec"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils"
)

func TestServices(t *testing.T) {
	stateDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, stateDir)

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

	initialize_stopBeforeClusterCreation(t)
	defer revert(t)

	t.Run("kill-services stops hub and agents", func(t *testing.T) {
		restartServices(t)
		processMustBeRunning(t, "gpupgrade hub")
		processMustBeRunning(t, "gpupgrade agent")

		killServices(t)
		processMustNotBeRunning(t, "gpupgrade hub")
		processMustNotBeRunning(t, "gpupgrade agent")
	})

	t.Run("kill-services stops hub and agents on default port if config file does not exist", func(t *testing.T) {
		restartServices(t)
		processMustBeRunning(t, "gpupgrade hub")
		processMustBeRunning(t, "gpupgrade agent")

		// move the gpupgrade state dir so that kill-services will use the default port
		tempStateDir := stateDir + ".bak"
		err := os.Rename(stateDir, tempStateDir)
		if err != nil {
			t.Fatalf("unexpected error: %#v", err)
		}
		defer func() {
			err = os.Rename(tempStateDir, stateDir)
			if err != nil {
				t.Fatalf("unexpected error: %#v", err)
			}
		}()

		killServices(t)
		processMustNotBeRunning(t, "gpupgrade hub")
		processMustNotBeRunning(t, "gpupgrade agent")
	})

	t.Run("restart-services actually starts hub and agents", func(t *testing.T) {
		killServices(t)
		processMustNotBeRunning(t, "gpupgrade hub")
		processMustNotBeRunning(t, "gpupgrade agent")

		restartServices(t)
		processMustBeRunning(t, "gpupgrade hub")
		processMustBeRunning(t, "gpupgrade agent")
	})

	t.Run("kill services can be run multiple times without issue", func(t *testing.T) {
		killServices(t)
		processMustNotBeRunning(t, "gpupgrade hub")
		processMustNotBeRunning(t, "gpupgrade agent")

		killServices(t)
		processMustNotBeRunning(t, "gpupgrade hub")
		processMustNotBeRunning(t, "gpupgrade agent")
	})

	t.Run("restart services can be run multiple times without issue", func(t *testing.T) {
		restartServices(t)
		processMustBeRunning(t, "gpupgrade hub")
		processMustBeRunning(t, "gpupgrade agent")

		restartServices(t)
		processMustBeRunning(t, "gpupgrade hub")
		processMustBeRunning(t, "gpupgrade agent")
	})
}

func processRunning(t *testing.T, process string) (bool, error) {
	t.Helper()

	cmd := exec.Command("pgrep", "-f", process)
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() == 1 {
			// No processes were matched
			return false, nil
		}
	}

	if err != nil {
		return false, err
	}

	return true, nil
}

func processMustBeRunning(t *testing.T, process string) {
	t.Helper()

	isRunning, err := processRunning(t, process)
	if err != nil {
		t.Fatalf("unexpected err: %#v", err)
	}

	if !isRunning {
		t.Fatalf("expected %q to be running", process)
	}
}

func processMustNotBeRunning(t *testing.T, process string) {
	t.Helper()

	isRunning, err := processRunning(t, process)
	if err != nil {
		t.Fatalf("unexpected err: %#v", err)
	}

	if isRunning {
		t.Fatalf("expected %q to not be running", process)
	}
}
