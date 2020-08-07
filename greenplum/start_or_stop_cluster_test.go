// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"errors"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestMain(m *testing.M) {
	os.Exit(exectest.Run(m))
}

func StartClusterCmd()        {}
func StopClusterCmd()         {}
func IsPostmasterRunningCmd() {}
func IsPostmasterRunningCmd_Errors() {
	os.Stderr.WriteString("exit status 2")
	os.Exit(2)
}
func IsPostmasterRunningCmd_MatchesNoProcesses() {
	os.Exit(1)
}

func init() {
	exectest.RegisterMains(
		StartClusterCmd,
		StopClusterCmd,
		IsPostmasterRunningCmd,
		IsPostmasterRunningCmd_Errors,
		IsPostmasterRunningCmd_MatchesNoProcesses,
	)
}

func TestStartOrStopCluster(t *testing.T) {
	testhelper.SetupTestLogger() // initialize gplog

	masterDataDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("creating temporary directory: %+v", err)
	}
	defer func() {
		err := os.RemoveAll(masterDataDir)
		if err != nil {
			t.Fatalf("removing temp dir %q: %#v", masterDataDir, err)
		}
	}()

	masterPidFile := filepath.Join(masterDataDir, "postmaster.pid")
	err = ioutil.WriteFile(masterPidFile, nil, 0600)
	if err != nil {
		t.Errorf("WriteFile returned error: %+v", err)
	}

	source := MustCreateCluster(t, []SegConfig{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: masterDataDir, Role: "p"},
	})
	source.GPHome = "/usr/local/source"

	utils.System.RemoveAll = func(s string) error { return nil }
	utils.System.MkdirAll = func(s string, perm os.FileMode) error { return nil }

	startStopCmd = nil
	isPostmasterRunningCmd = nil

	defer func() {
		startStopCmd = exec.Command
		isPostmasterRunningCmd = exec.Command
	}()

	t.Run("IsMasterRunning succeeds", func(t *testing.T) {
		isPostmasterRunningCmd = exectest.NewCommandWithVerifier(IsPostmasterRunningCmd,
			func(path string, args ...string) {
				if path != "pgrep" {
					t.Errorf("got %q want pgrep", path)
				}

				expected := []string{"-F", masterPidFile}
				if !reflect.DeepEqual(args, expected) {
					t.Errorf("got %q want %q", args, expected)
				}
			})

		running, err := source.IsMasterRunning(step.DevNullStream)
		if err != nil {
			t.Errorf("IsMasterRunning returned error: %+v", err)
		}

		if !running {
			t.Error("expected postmaster to be running")
		}
	})

	t.Run("IsMasterRunning fails", func(t *testing.T) {
		isPostmasterRunningCmd = exectest.NewCommand(IsPostmasterRunningCmd_Errors)

		running, err := source.IsMasterRunning(step.DevNullStream)
		var expected *exec.ExitError
		if !errors.As(err, &expected) {
			t.Errorf("expected error to contain type %T", expected)
		}

		if running {
			t.Error("expected postmaster to not be running")
		}
	})

	t.Run("returns false with no error when master data directory does not exist", func(t *testing.T) {
		source := MustCreateCluster(t, []SegConfig{
			{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/does/not/exist", Role: "p"},
		})
		running, err := source.IsMasterRunning(step.DevNullStream)
		if err != nil {
			t.Errorf("IsMasterRunning returned error: %+v", err)
		}

		if running {
			t.Error("expected postmaster to not be running")
		}
	})

	t.Run("returns false with no error when no processes were matched", func(t *testing.T) {
		isPostmasterRunningCmd = exectest.NewCommand(IsPostmasterRunningCmd_MatchesNoProcesses)

		running, err := source.IsMasterRunning(step.DevNullStream)
		if err != nil {
			t.Errorf("IsMasterRunning returned error: %+v", err)
		}

		if running {
			t.Error("expected postmaster to not be running")
		}
	})

	t.Run("stop cluster successfully shuts down cluster", func(t *testing.T) {
		isPostmasterRunningCmd = exectest.NewCommandWithVerifier(IsPostmasterRunningCmd,
			func(path string, args ...string) {
				if path != "pgrep" {
					t.Errorf("got %q want pgrep", path)
				}

				expected := []string{"-F", masterPidFile}
				if !reflect.DeepEqual(args, expected) {
					t.Errorf("got %q want %q", args, expected)
				}
			})

		startStopCmd = exectest.NewCommandWithVerifier(StopClusterCmd,
			func(path string, args ...string) {
				if path != "bash" {
					t.Errorf("got %q want bash", path)
				}

				expected := []string{"-c", "source /usr/local/source/greenplum_path.sh " +
					"&& MASTER_DATA_DIRECTORY=" + masterDataDir + " /usr/local/source/bin/gpstop -a -d " + masterDataDir}
				if !reflect.DeepEqual(args, expected) {
					t.Errorf("got %q want %q", args, expected)
				}
			})

		err := source.Stop(step.DevNullStream)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}
	})

	t.Run("stop cluster detects that cluster is already shutdown", func(t *testing.T) {
		isPostmasterRunningCmd = exectest.NewCommand(IsPostmasterRunningCmd_Errors)

		var skippedStopClusterCommand = true
		startStopCmd = exectest.NewCommandWithVerifier(IsPostmasterRunningCmd,
			func(path string, args ...string) {
				skippedStopClusterCommand = false
			})

		err := source.Stop(step.DevNullStream)
		if err == nil {
			t.Errorf("expected error %#v got nil", err)
		}

		if !skippedStopClusterCommand {
			t.Error("expected skippedStopClusterCommand to be true")
		}
	})

	t.Run("start cluster successfully starts up cluster", func(t *testing.T) {
		startStopCmd = exectest.NewCommandWithVerifier(StartClusterCmd,
			func(path string, args ...string) {
				if path != "bash" {
					t.Errorf("got %q want bash", path)
				}

				expected := []string{"-c", "source /usr/local/source/greenplum_path.sh " +
					"&& MASTER_DATA_DIRECTORY=" + masterDataDir + " /usr/local/source/bin/gpstart -a -d " + masterDataDir}
				if !reflect.DeepEqual(args, expected) {
					t.Errorf("got %q want %q", args, expected)
				}
			})

		err := source.Start(step.DevNullStream)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}
	})

	t.Run("start master successfully starts up master only", func(t *testing.T) {
		startStopCmd = exectest.NewCommandWithVerifier(StartClusterCmd,
			func(path string, args ...string) {
				if path != "bash" {
					t.Errorf("got %q want bash", path)
				}

				expected := []string{"-c", "source /usr/local/source/greenplum_path.sh " +
					"&& MASTER_DATA_DIRECTORY=" + masterDataDir + " /usr/local/source/bin/gpstart -m -a -d " + masterDataDir}
				if !reflect.DeepEqual(args, expected) {
					t.Errorf("got %q want %q", args, expected)
				}
			})

		err := source.StartMasterOnly(step.DevNullStream)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}
	})

	t.Run("stop master successfully shuts down master only", func(t *testing.T) {
		isPostmasterRunningCmd = exectest.NewCommandWithVerifier(IsPostmasterRunningCmd,
			func(path string, args ...string) {
				if path != "pgrep" {
					t.Errorf("got %q want pgrep", path)
				}

				expected := []string{"-F", masterPidFile}
				if !reflect.DeepEqual(args, expected) {
					t.Errorf("got %q want %q", args, expected)
				}
			})

		startStopCmd = exectest.NewCommandWithVerifier(StopClusterCmd,
			func(path string, args ...string) {
				if path != "bash" {
					t.Errorf("got %q want bash", path)
				}

				expected := []string{"-c", "source /usr/local/source/greenplum_path.sh " +
					"&& MASTER_DATA_DIRECTORY=" + masterDataDir + " /usr/local/source/bin/gpstop -m -a -d " + masterDataDir}
				if !reflect.DeepEqual(args, expected) {
					t.Errorf("got %q want %q", args, expected)
				}
			})

		err := source.StopMasterOnly(step.DevNullStream)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}
	})
}
