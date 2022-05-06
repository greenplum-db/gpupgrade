// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum_test

import (
	"errors"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
)

func TestStart(t *testing.T) {
	testlog.SetupLogger()

	dataDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, dataDir)

	source := greenplum.MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: dataDir, Role: greenplum.PrimaryRole},
	})
	source.GPHome = "/usr/local/source"
	source.Destination = idl.ClusterDestination_SOURCE

	t.Run("start succeeds", func(t *testing.T) {
		cmd := exectest.NewCommandWithVerifier(Success, func(name string, args ...string) {
			expected := "bash"
			if name != expected {
				t.Errorf("got %q want %q", name, expected)
			}

			expectedArgs := []string{"-c", "source /usr/local/source/greenplum_path.sh && /usr/local/source/bin/gpstart -a -d " + dataDir}
			if !reflect.DeepEqual(args, expectedArgs) {
				t.Errorf("got %q want %q", args, expectedArgs)
			}
		})
		greenplum.SetGreenplumCommand(cmd)
		defer greenplum.ResetGreenplumCommand()

		err := source.Start(step.DevNullStream)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("start returns errors", func(t *testing.T) {
		greenplum.SetGreenplumCommand(exectest.NewCommand(FailedMain))
		defer greenplum.ResetGreenplumCommand()

		err := source.Start(step.DevNullStream)
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Errorf("got %T, want %T", err, exitError)
		}

		expected := "starting source cluster: exit status 1"
		if err.Error() != expected {
			t.Errorf("got %q want %q", err.Error(), expected)
		}
	})
}

func TestStartMasterOnly(t *testing.T) {
	testlog.SetupLogger()

	dataDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, dataDir)

	source := greenplum.MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: dataDir, Role: greenplum.PrimaryRole},
	})
	source.GPHome = "/usr/local/source"
	source.Destination = idl.ClusterDestination_SOURCE

	t.Run("start master only succeeds", func(t *testing.T) {
		cmd := exectest.NewCommandWithVerifier(Success, func(name string, args ...string) {
			expected := "bash"
			if name != expected {
				t.Errorf("got %q want %q", name, expected)
			}

			expectedArgs := []string{"-c", "source /usr/local/source/greenplum_path.sh && /usr/local/source/bin/gpstart -a -m -d " + dataDir}
			if !reflect.DeepEqual(args, expectedArgs) {
				t.Errorf("got %q want %q", args, expectedArgs)
			}
		})
		greenplum.SetGreenplumCommand(cmd)
		defer greenplum.ResetGreenplumCommand()

		err := source.StartMasterOnly(step.DevNullStream)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("start master only returns errors", func(t *testing.T) {
		greenplum.SetGreenplumCommand(exectest.NewCommand(FailedMain))
		defer greenplum.ResetGreenplumCommand()

		err := source.StartMasterOnly(step.DevNullStream)
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Errorf("got %T, want %T", err, exitError)
		}

		expected := "starting source cluster in master only mode: exit status 1"
		if err.Error() != expected {
			t.Errorf("got %q want %q", err.Error(), expected)
		}
	})
}

func TestStop(t *testing.T) {
	testlog.SetupLogger()

	dataDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, dataDir)

	source := greenplum.MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: dataDir, Role: greenplum.PrimaryRole},
	})
	source.GPHome = "/usr/local/source"
	source.Destination = idl.ClusterDestination_SOURCE

	t.Run("stop succeeds", func(t *testing.T) {
		testutils.MustWriteToFile(t, filepath.Join(dataDir, "postmaster.pid"), "")

		cmd := exectest.NewCommandWithVerifier(Success, func(name string, args ...string) {
			expected := "pgrep"
			if name != expected {
				t.Errorf("got %q want %q", name, expected)
			}

			expectedArgs := []string{"-F", filepath.Join(dataDir, "postmaster.pid")}
			if !reflect.DeepEqual(args, expectedArgs) {
				t.Errorf("got %q want %q", args, expectedArgs)
			}
		})
		greenplum.SetIsMasterRunningCommand(cmd)
		defer greenplum.ResetIsMasterRunningCommand()

		cmd = exectest.NewCommandWithVerifier(Success, func(name string, args ...string) {
			expected := "bash"
			if name != expected {
				t.Errorf("got %q want %q", name, expected)
			}

			expectedArgs := []string{"-c", "source /usr/local/source/greenplum_path.sh && /usr/local/source/bin/gpstop -a -d " + dataDir}
			if !reflect.DeepEqual(args, expectedArgs) {
				t.Errorf("got %q want %q", args, expectedArgs)
			}
		})
		greenplum.SetGreenplumCommand(cmd)
		defer greenplum.ResetGreenplumCommand()

		err := source.Stop(step.DevNullStream)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("stop returns errors", func(t *testing.T) {
		testutils.MustWriteToFile(t, filepath.Join(dataDir, "postmaster.pid"), "")

		greenplum.SetIsMasterRunningCommand(exectest.NewCommand(Success))
		defer greenplum.ResetIsMasterRunningCommand()

		greenplum.SetGreenplumCommand(exectest.NewCommand(FailedMain))
		defer greenplum.ResetGreenplumCommand()

		err := source.Stop(step.DevNullStream)
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Errorf("got %T, want %T", err, exitError)
		}

		expected := "stopping source cluster: exit status 1"
		if err.Error() != expected {
			t.Errorf("got %q want %q", err.Error(), expected)
		}
	})

	t.Run("stop detects if the cluster is already shutdown", func(t *testing.T) {
		testutils.MustWriteToFile(t, filepath.Join(dataDir, "postmaster.pid"), "")

		greenplum.SetIsMasterRunningCommand(exectest.NewCommand(IsPostmasterRunningCmd_MatchesNoProcesses))
		defer greenplum.ResetIsMasterRunningCommand()

		err := source.Stop(step.DevNullStream)
		expected := "Failed to stop source cluster. Master is already stopped."
		if err.Error() != expected {
			t.Errorf("got %q want %q", err.Error(), expected)
		}
	})
}

func TestStopMasterOnly(t *testing.T) {
	testlog.SetupLogger()

	dataDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, dataDir)

	source := greenplum.MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: dataDir, Role: greenplum.PrimaryRole},
	})
	source.GPHome = "/usr/local/source"
	source.Destination = idl.ClusterDestination_SOURCE

	t.Run("stop master only succeeds", func(t *testing.T) {
		testutils.MustWriteToFile(t, filepath.Join(dataDir, "postmaster.pid"), "")

		cmd := exectest.NewCommandWithVerifier(Success, func(name string, args ...string) {
			expected := "pgrep"
			if name != expected {
				t.Errorf("got %q want %q", name, expected)
			}

			expectedArgs := []string{"-F", filepath.Join(dataDir, "postmaster.pid")}
			if !reflect.DeepEqual(args, expectedArgs) {
				t.Errorf("got %q want %q", args, expectedArgs)
			}
		})
		greenplum.SetIsMasterRunningCommand(cmd)
		defer greenplum.ResetIsMasterRunningCommand()

		cmd = exectest.NewCommandWithVerifier(Success, func(name string, args ...string) {
			expected := "bash"
			if name != expected {
				t.Errorf("got %q want %q", name, expected)
			}

			expectedArgs := []string{"-c", "source /usr/local/source/greenplum_path.sh && /usr/local/source/bin/gpstop -a -m -d " + dataDir}
			if !reflect.DeepEqual(args, expectedArgs) {
				t.Errorf("got %q want %q", args, expectedArgs)
			}
		})
		greenplum.SetGreenplumCommand(cmd)
		defer greenplum.ResetGreenplumCommand()

		err := source.StopMasterOnly(step.DevNullStream)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("stop master only returns errors", func(t *testing.T) {
		testutils.MustWriteToFile(t, filepath.Join(dataDir, "postmaster.pid"), "")

		greenplum.SetIsMasterRunningCommand(exectest.NewCommand(Success))
		defer greenplum.ResetIsMasterRunningCommand()

		greenplum.SetGreenplumCommand(exectest.NewCommand(FailedMain))
		defer greenplum.ResetGreenplumCommand()

		err := source.StopMasterOnly(step.DevNullStream)
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Errorf("got %T, want %T", err, exitError)
		}

		expected := "stopping source cluster: exit status 1"
		if err.Error() != expected {
			t.Errorf("got %q want %q", err.Error(), expected)
		}
	})

	t.Run("stop master only detects if the cluster is already shutdown", func(t *testing.T) {
		testutils.MustWriteToFile(t, filepath.Join(dataDir, "postmaster.pid"), "")

		greenplum.SetIsMasterRunningCommand(exectest.NewCommand(IsPostmasterRunningCmd_MatchesNoProcesses))
		defer greenplum.ResetIsMasterRunningCommand()

		err := source.StopMasterOnly(step.DevNullStream)
		expected := "Failed to stop source cluster in master only mode. Master is already stopped."
		if err.Error() != expected {
			t.Errorf("got %q want %q", err.Error(), expected)
		}
	})
}

func TestIsMasterRunning(t *testing.T) {
	testlog.SetupLogger()

	dataDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, dataDir)

	source := greenplum.MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: dataDir, Role: greenplum.PrimaryRole},
	})
	source.GPHome = "/usr/local/source"
	source.Destination = idl.ClusterDestination_SOURCE

	t.Run("IsMasterRunning succeeds", func(t *testing.T) {
		testutils.MustWriteToFile(t, filepath.Join(dataDir, "postmaster.pid"), "")

		greenplum.SetIsMasterRunningCommand(exectest.NewCommand(Success))
		defer greenplum.ResetIsMasterRunningCommand()

		running, err := source.IsMasterRunning(step.DevNullStream)
		if err != nil {
			t.Errorf("IsMasterRunning returned error: %+v", err)
		}

		if !running {
			t.Error("expected postmaster to be running")
		}
	})

	t.Run("IsMasterRunning returns errors", func(t *testing.T) {
		testutils.MustWriteToFile(t, filepath.Join(dataDir, "postmaster.pid"), "")

		greenplum.SetIsMasterRunningCommand(exectest.NewCommand(IsPostmasterRunningCmd_Errors))
		defer greenplum.ResetIsMasterRunningCommand()

		running, err := source.IsMasterRunning(step.DevNullStream)
		var expected *exec.ExitError
		if !errors.As(err, &expected) {
			t.Errorf("expected error to contain type %T", expected)
		}

		if running {
			t.Error("expected postmaster to not be running")
		}
	})

	t.Run("IsMasterRunning returns false with no error when master data directory does not exist", func(t *testing.T) {
		source := greenplum.MustCreateCluster(t, greenplum.SegConfigs{
			{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/does/not/exist", Role: greenplum.PrimaryRole},
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
		testutils.MustWriteToFile(t, filepath.Join(dataDir, "postmaster.pid"), "")

		greenplum.SetIsMasterRunningCommand(exectest.NewCommand(IsPostmasterRunningCmd_MatchesNoProcesses))
		defer greenplum.ResetIsMasterRunningCommand()

		running, err := source.IsMasterRunning(step.DevNullStream)
		if err != nil {
			t.Errorf("IsMasterRunning returned error: %+v", err)
		}

		if running {
			t.Error("expected postmaster to not be running")
		}
	})
}
