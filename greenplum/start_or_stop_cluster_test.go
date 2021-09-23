// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum_test

import (
	"errors"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestMain(m *testing.M) {
	os.Exit(exectest.Run(m))
}

func IsPostmasterRunningCmd_Errors() {
	os.Stderr.WriteString("exit status 2")
	os.Exit(2)
}
func IsPostmasterRunningCmd_MatchesNoProcesses() {
	os.Exit(1)
}

func init() {
	exectest.RegisterMains(
		IsPostmasterRunningCmd_Errors,
		IsPostmasterRunningCmd_MatchesNoProcesses,
	)
}

func TestStartOrStopCluster(t *testing.T) {
	testlog.SetupLogger()

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

	source := greenplum.MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: masterDataDir, Role: greenplum.PrimaryRole},
	})
	source.GPHome = "/usr/local/source"

	utils.System.RemoveAll = func(s string) error { return nil }
	utils.System.MkdirAll = func(s string, perm os.FileMode) error { return nil }

	t.Run("IsMasterRunning succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mock, cleanup := greenplum.MockExecCommand(ctrl)
		defer cleanup()

		// A successful pgrep implies a running postmaster.
		mock.EXPECT().Command("pgrep", []string{"-F", masterPidFile})

		running, err := source.IsMasterRunning(step.DevNullStream)
		if err != nil {
			t.Errorf("IsMasterRunning returned error: %+v", err)
		}

		if !running {
			t.Error("expected postmaster to be running")
		}
	})

	t.Run("IsMasterRunning fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mock, cleanup := greenplum.MockExecCommand(ctrl)
		defer cleanup()

		// An error in pgrep should be bubbled up.
		mock.EXPECT().Command("pgrep", gomock.Any()).
			Return(IsPostmasterRunningCmd_Errors)

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
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mock, cleanup := greenplum.MockExecCommand(ctrl)
		defer cleanup()

		mock.EXPECT().Command("pgrep", gomock.Any()).
			Return(IsPostmasterRunningCmd_MatchesNoProcesses)

		running, err := source.IsMasterRunning(step.DevNullStream)
		if err != nil {
			t.Errorf("IsMasterRunning returned error: %+v", err)
		}

		if running {
			t.Error("expected postmaster to not be running")
		}
	})

	t.Run("stop cluster successfully shuts down cluster", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mock, cleanup := greenplum.MockExecCommand(ctrl)
		defer cleanup()

		script := "source /usr/local/source/greenplum_path.sh " +
			"&& MASTER_DATA_DIRECTORY=" + masterDataDir + " /usr/local/source/bin/gpstop -a -d " + masterDataDir

		mock.EXPECT().Command("pgrep", []string{"-F", masterPidFile})
		mock.EXPECT().Command("bash", []string{"-c", script})

		err := source.Stop(step.DevNullStream)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}
	})

	t.Run("stop cluster detects that cluster is already shutdown", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mock, cleanup := greenplum.MockExecCommand(ctrl)
		defer cleanup()

		mock.EXPECT().Command("pgrep", gomock.Any()).
			Return(IsPostmasterRunningCmd_MatchesNoProcesses)
		// We expect no bash invocations.

		err := source.Stop(step.DevNullStream)
		if err == nil {
			t.Errorf("expected error %#v got nil", err)
		}
	})

	t.Run("start cluster successfully starts up cluster", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mock, cleanup := greenplum.MockExecCommand(ctrl)
		defer cleanup()

		script := "source /usr/local/source/greenplum_path.sh " +
			"&& MASTER_DATA_DIRECTORY=" + masterDataDir + " /usr/local/source/bin/gpstart -a -d " + masterDataDir

		mock.EXPECT().Command("bash", []string{"-c", script})

		err := source.Start(step.DevNullStream)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}
	})

	t.Run("start master successfully starts up master only", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mock, cleanup := greenplum.MockExecCommand(ctrl)
		defer cleanup()

		script := "source /usr/local/source/greenplum_path.sh " +
			"&& MASTER_DATA_DIRECTORY=" + masterDataDir + " /usr/local/source/bin/gpstart -m -a -d " + masterDataDir

		mock.EXPECT().Command("bash", []string{"-c", script})

		err := source.StartMasterOnly(step.DevNullStream)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}
	})

	t.Run("stop master successfully shuts down master only", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mock, cleanup := greenplum.MockExecCommand(ctrl)
		defer cleanup()

		script := "source /usr/local/source/greenplum_path.sh " +
			"&& MASTER_DATA_DIRECTORY=" + masterDataDir + " /usr/local/source/bin/gpstop -m -a -d " + masterDataDir

		mock.EXPECT().Command("pgrep", []string{"-F", masterPidFile})
		mock.EXPECT().Command("bash", []string{"-c", script})

		err := source.StopMasterOnly(step.DevNullStream)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}
	})
}
