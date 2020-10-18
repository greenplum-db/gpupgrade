// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commanders

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

// Streams the above stdout/err constants to the corresponding standard file
// descriptors, alternately interleaving five-byte chunks.
func IsHubRunning_True() {
	fmt.Print("1")
	os.Exit(0)
}

func IsHubRunning_False() {
	fmt.Print("0")
	os.Exit(1)
}

func IsHubRunning_Error() {
	fmt.Print("bengie")
	os.Exit(2)
}

func GpupgradeHub_good_Main() {
	fmt.Print("Hi, Hub started.")
}

func GpupgradeHub_bad_Main() {
	fmt.Fprint(os.Stderr, "Sorry, Hub could not be started.")
	os.Exit(1)
}

func init() {
	exectest.RegisterMains(
		IsHubRunning_True,
		IsHubRunning_False,
		IsHubRunning_Error,
		GpupgradeHub_good_Main,
		GpupgradeHub_bad_Main,
	)
}

func setup(t *testing.T) {
	execCommandHubStart = nil
	execCommandHubCount = nil
}

func teardown() {
	execCommandHubStart = exec.Command
	execCommandHubCount = exec.Command
}

func TestIsHubRunning_ReturnsFalseWhenNotRunning(t *testing.T) {
	setup(t)
	defer teardown()

	execCommandHubCount = exectest.NewCommand(IsHubRunning_False)
	running, err := IsHubRunning()
	if err != nil {
		t.Errorf("unexpected error %#v", err)
	}

	if running {
		t.Error("expected running to be false")
	}
}

func TestIsHubRunning_ReturnsTrueWhenRunning(t *testing.T) {
	setup(t)
	defer teardown()

	execCommandHubCount = exectest.NewCommand(IsHubRunning_True)
	running, err := IsHubRunning()
	if err != nil {
		t.Errorf("unexpected error %#v", err)
	}

	if !running {
		t.Error("expected running to be true")
	}
}

func TestIsHubRunning_ErrorsWhenCheckFails(t *testing.T) {
	setup(t)
	defer teardown()

	execCommandHubCount = exectest.NewCommand(IsHubRunning_Error)
	running, err := IsHubRunning()
	var expected *exec.ExitError
	if !errors.As(err, &expected) {
		t.Errorf("returned error %#v want %#v", err, expected)
	}

	if running {
		t.Error("expected running to be false")
	}
}

func TestStartHub_Succeeds(t *testing.T) {
	setup(t)
	defer teardown()

	execCommandHubCount = exectest.NewCommand(IsHubRunning_False)
	execCommandHubStart = exectest.NewCommand(GpupgradeHub_good_Main)
	err := StartHub()
	if err != nil {
		t.Errorf("unexpected error %#v", err)
	}
}

func TestStartHub_FailsToStartWhenHubIsRunningErrors(t *testing.T) {
	setup(t)
	defer teardown()

	execCommandHubCount = exectest.NewCommand(IsHubRunning_Error)
	execCommandHubStart = exectest.NewCommand(GpupgradeHub_good_Main) // should not hit this, but fail it we do
	err := StartHub()
	var expected *exec.ExitError
	if !errors.As(err, &expected) {
		t.Errorf("returned error %#v want %#v", err, expected)
	}
}

func TestStartHub_IsSkippedWhenHubIsRunning(t *testing.T) {
	setup(t)
	defer teardown()

	execCommandHubCount = exectest.NewCommand(IsHubRunning_True)
	execCommandHubStart = exectest.NewCommand(GpupgradeHub_bad_Main) // should not hit this, but fail if we do
	err := StartHub()

	if !errors.Is(err, step.Skip) {
		t.Errorf("unexpected error %#v", err)
	}
}

func TestStartHub_FailsWhenStartingTheHubErrors(t *testing.T) {
	setup(t)
	defer teardown()

	execCommandHubCount = exectest.NewCommand(IsHubRunning_False)
	execCommandHubStart = exectest.NewCommand(GpupgradeHub_bad_Main)
	err := StartHub()
	if err == nil {
		t.Errorf("expected error %#v got nil", err)
	}
}

func TestCreateStateDir(t *testing.T) {
	home, err := ioutil.TempDir("", t.Name())
	if err != nil {
		t.Fatalf("failed creating temp dir %#v", err)
	}

	oldStateDir, isSet := os.LookupEnv("GPUGRADE_HOME")
	defer func() {
		if isSet {
			os.Setenv("GPUPGRADE_HOME", oldStateDir)
		}
	}()

	stateDir := filepath.Join(home, ".gpupgrade")
	err = os.Setenv("GPUPGRADE_HOME", stateDir)
	if err != nil {
		t.Fatalf("failed to set GPUPGRADE_HOME %#v", err)
	}

	t.Run("test idempotence", func(t *testing.T) {
		var infoOld os.FileInfo

		{ // creates state directory if none exist or fails
			if _, err = os.Stat(stateDir); err == nil {
				t.Errorf("stateDir exists")
			}

			err = CreateStateDir()
			if err != nil {
				t.Fatalf("unexpected error %#v", err)
			}

			if infoOld, err = os.Stat(home); err != nil {
				t.Errorf("unexpected error %#v", err)
			}
		}

		{ // creating state directory is idempotent
			err = CreateStateDir()
			if err != nil {
				t.Fatalf("unexpected error %#v", err)
			}

			var infoNew os.FileInfo
			if infoNew, err = os.Stat(home); err != nil {
				t.Errorf("unexpected error %#v", err)
			}

			if !reflect.DeepEqual(infoOld, infoNew) {
				t.Error("want fileInfo before to match fileInfo new")
			}
		}

		{ //  creating state directory succeeds on multiple runs
			err = CreateStateDir()
			if err != nil {
				t.Fatalf("unexpected error %#v", err)
			}
		}
	})
}

func TestCreateInitialClusterConfigs(t *testing.T) {
	const port = -1

	home, err := ioutil.TempDir("", t.Name())
	if err != nil {
		t.Fatalf("failed creating temp dir %#v", err)
	}

	oldStateDir, isSet := os.LookupEnv("GPUGRADE_HOME")
	defer func() {
		if isSet {
			os.Setenv("GPUPGRADE_HOME", oldStateDir)
		}
	}()
	stateDir := filepath.Join(home, ".gpupgrade")
	err = os.Setenv("GPUPGRADE_HOME", stateDir)
	if err != nil {
		t.Fatalf("failed to set GPUPGRADE_HOME %#v", err)
	}

	if _, err := os.Stat(stateDir); err == nil {
		t.Errorf("stateDir exists")
	}
	err = CreateStateDir()
	if err != nil {
		t.Fatalf("failed to create state dir %#v", err)
	}

	var sourceOld os.FileInfo

	t.Run("test idempotence", func(t *testing.T) {

		{ // creates initial cluster config files if none exist or fails"
			err = CreateInitialClusterConfigs(port)
			if err != nil {
				t.Fatalf("unexpected error %#v", err)
			}

			if sourceOld, err = os.Stat(upgrade.GetConfigFile()); err != nil {
				t.Errorf("unexpected error %#v", err)
			}
		}

		{ // creating cluster config files is idempotent
			err = CreateInitialClusterConfigs(port)
			if err != nil {
				t.Fatalf("unexpected error %#v", err)
			}

			var sourceNew os.FileInfo
			if sourceNew, err = os.Stat(upgrade.GetConfigFile()); err != nil {
				t.Errorf("got unexpected error %#v", err)
			}

			if sourceOld.ModTime() != sourceNew.ModTime() {
				t.Errorf("want %#v got %#v", sourceOld.ModTime(), sourceNew.ModTime())
			}
		}

		{ // creating cluster config files succeeds on multiple runs
			err = CreateInitialClusterConfigs(port)
			if err != nil {
				t.Fatalf("unexpected error %#v", err)
			}
		}
	})
}
