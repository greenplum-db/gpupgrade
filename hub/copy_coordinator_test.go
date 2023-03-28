// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/config/backupdir"
	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

const (
	rsyncExitCode     int    = 23 // rsync returns 23 for a partial transfer
	rsyncErrorMessage string = `rsync: recv_generator: mkdir "/tmp/coordinator_copy/gpseg-1" failed: Permission denied(13)
*** Skipping any contents from this failed directory ***
rsync error: some files/attrs were not transferred (see previous errors) (code 23) atmain.c(1052) [sender=3.0.9]
`
)

func RsyncFailure() {
	fmt.Fprint(os.Stderr, rsyncErrorMessage)
	os.Exit(rsyncExitCode)
}

func init() {
	exectest.RegisterMains(
		RsyncFailure,
	)
}

func TestCopy(t *testing.T) {
	testlog.SetupTestLogger()

	t.Run("copies the directory only once per host", func(t *testing.T) {
		sourceDirs := []string{"/data/qddir/seg-1/"}

		backupDirs := backupdir.BackupDirs{}
		backupDirs.AgentHostsToBackupDir = make(backupdir.AgentHostsToBackupDir)
		backupDirs.AgentHostsToBackupDir["localhost"] = "foobar/path"

		cmd := exectest.NewCommandWithVerifier(hub.Success, func(name string, args ...string) {
			expected := "rsync"
			if !strings.HasSuffix(name, expected) {
				t.Errorf("got %q, want %q", name, expected)
			}

			expectedArgs := []string{
				"--archive", "--compress", "--delete", "--stats",
				"/data/qddir/seg-1/", "localhost:foobar/path",
			}
			if !reflect.DeepEqual(args, expectedArgs) {
				t.Errorf("rsync invoked with %q, want %q", args, expectedArgs)
			}
		})
		rsync.SetRsyncCommand(cmd)
		defer rsync.ResetRsyncCommand()

		err := hub.Copy(step.DevNullStream, sourceDirs, backupDirs.AgentHostsToBackupDir)
		if err != nil {
			t.Errorf("copying data directory: %+v", err)
		}
	})

	t.Run("copies the data directory to each host", func(t *testing.T) {
		sourceDirs := []string{"/data/qddir/seg-1"}

		backupDirs := backupdir.BackupDirs{}
		backupDirs.AgentHostsToBackupDir = make(backupdir.AgentHostsToBackupDir)
		backupDirs.AgentHostsToBackupDir["host1"] = "foobar1/path"
		backupDirs.AgentHostsToBackupDir["host2"] = "foobar2/path"

		actualArgsChan := make(chan []string, len(backupDirs.AgentHostsToBackupDir))

		cmd := exectest.NewCommandWithVerifier(hub.Success, func(name string, args ...string) {
			expected := "rsync"
			if !strings.HasSuffix(name, expected) {
				t.Errorf("got %q, want %q", name, expected)
			}

			actualArgsChan <- args
		})
		rsync.SetRsyncCommand(cmd)
		defer rsync.ResetRsyncCommand()

		err := hub.Copy(step.DevNullStream, sourceDirs, backupDirs.AgentHostsToBackupDir)
		if err != nil {
			t.Errorf("copying directory: %+v", err)
		}

		close(actualArgsChan)

		var expectedArgs Args
		for host, backupDir := range backupDirs.AgentHostsToBackupDir {
			expectedArgs = append(expectedArgs, []string{
				"--archive", "--compress", "--delete", "--stats",
				"/data/qddir/seg-1", fmt.Sprintf("%s:%s", host, backupDir)})
		}

		verifyArgs(t, actualArgsChan, expectedArgs)
	})

	t.Run("returns errors when writing stdout and stderr buffers to the stream", func(t *testing.T) {
		backupDirs := backupdir.BackupDirs{}
		backupDirs.AgentHostsToBackupDir = make(backupdir.AgentHostsToBackupDir)
		backupDirs.AgentHostsToBackupDir["localhost"] = "foobar/path"

		streams := testutils.FailingStreams{Err: errors.New("e")}

		rsync.SetRsyncCommand(exectest.NewCommand(hub.StreamingMain))
		defer rsync.ResetRsyncCommand()

		err := hub.Copy(streams, []string{""}, backupDirs.AgentHostsToBackupDir)

		var errs errorlist.Errors
		if !errors.As(err, &errs) {
			t.Fatalf("returned %#v, want error type %T", err, errs)
		}

		for _, err := range errs {
			if !errors.Is(err, streams.Err) {
				t.Errorf("returned error %#v, want %#v", err, streams.Err)
			}
		}
	})

	t.Run("serializes rsync failures to the log stream", func(t *testing.T) {
		backupDirs := backupdir.BackupDirs{}
		backupDirs.AgentHostsToBackupDir = make(backupdir.AgentHostsToBackupDir)
		backupDirs.AgentHostsToBackupDir["sdw1"] = "foobar1/path"
		backupDirs.AgentHostsToBackupDir["sdw2"] = "foobar2/path"

		buffer := new(step.BufferedStreams)

		rsync.SetRsyncCommand(exectest.NewCommand(RsyncFailure))
		defer rsync.ResetRsyncCommand()

		err := hub.Copy(buffer, []string{"data/coordinator"}, backupDirs.AgentHostsToBackupDir)

		var errs errorlist.Errors
		if !errors.As(err, &errs) {
			t.Fatalf("returned %#v, want error type %T", err, errs)
		}

		var exitErr *exec.ExitError
		for _, err := range errs {
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != rsyncExitCode {
				t.Errorf("returned error %#v, want exit code %d", err, rsyncExitCode)
			}
		}

		stdout := buffer.StdoutBuf.String()
		if len(stdout) != 0 {
			t.Errorf("got stdout %q, expected no output", stdout)
		}

		// Make sure we have as many copies of the stderr string as there are
		// hosts. They should be serialized sanely, even though we may execute
		// in parallel.
		stderr := buffer.StderrBuf.String()
		expected := strings.Repeat(rsyncErrorMessage, len(backupDirs.AgentHostsToBackupDir))
		if stderr != expected {
			t.Errorf("got stderr:\n%v\nwant:\n%v", stderr, expected)
		}
	})
}

func TestCopyCoordinatorDataDir(t *testing.T) {
	testlog.SetupTestLogger()

	intermediate := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
	})

	backupDirs, err := hub.ParseParentBackupDirs("", intermediate)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("copies the coordinator data directory to each primary host", func(t *testing.T) {
		actualArgsChan := make(chan []string, len(backupDirs.AgentHostsToBackupDir))

		cmd := exectest.NewCommandWithVerifier(hub.Success, func(name string, args ...string) {
			expected := "rsync"
			if !strings.HasSuffix(name, expected) {
				t.Errorf("got %q, want %q", name, expected)
			}

			actualArgsChan <- args
		})
		rsync.SetRsyncCommand(cmd)
		defer rsync.ResetRsyncCommand()

		err := hub.CopyCoordinatorDataDir(step.DevNullStream, intermediate.CoordinatorDataDir(), backupDirs.AgentHostsToBackupDir)
		if err != nil {
			t.Errorf("copying coordinator data directory: %+v", err)
		}

		close(actualArgsChan)

		var expectedArgs Args
		for host, backupDir := range backupDirs.AgentHostsToBackupDir {
			expectedArgs = append(expectedArgs, []string{
				"--archive", "--compress", "--delete", "--stats",
				intermediate.CoordinatorDataDir() + string(os.PathSeparator),
				fmt.Sprintf("%s:%s", host, utils.GetCoordinatorPostUpgradeBackupDir(backupDir))})
		}

		verifyArgs(t, actualArgsChan, expectedArgs)
	})
}

func TestCopyCoordinatorTablespaces(t *testing.T) {
	testlog.SetupTestLogger()

	stateDir := testutils.GetTempDir(t, "")
	defer os.RemoveAll(stateDir)

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

	intermediate := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
	})

	backupDirs, err := hub.ParseParentBackupDirs("", intermediate)
	if err != nil {
		t.Fatal(err)
	}

	Tablespaces := greenplum.Tablespaces{
		1: greenplum.SegmentTablespaces{
			1663: &idl.TablespaceInfo{
				Location:    "/tmp/tblspc1",
				UserDefined: false},
			1664: &idl.TablespaceInfo{
				Location:    "/tmp/tblspc2",
				UserDefined: true},
		},
		2: greenplum.SegmentTablespaces{
			1663: &idl.TablespaceInfo{
				Location:    "/tmp/primary1/tblspc1",
				UserDefined: false},
			1664: &idl.TablespaceInfo{
				Location:    "/tmp/primary1/tblspc2",
				UserDefined: true},
		},
		3: greenplum.SegmentTablespaces{
			1663: &idl.TablespaceInfo{
				Location:    "/tmp/primary2/tblspc1",
				UserDefined: false},
			1664: &idl.TablespaceInfo{
				Location:    "/tmp/primary2/tblspc2",
				UserDefined: true},
		},
	}

	t.Run("when source version is 5X it copy's the --old-tablespace-file and user defined coordinator tablespace locations to each primary host", func(t *testing.T) {
		actualArgsChan := make(chan []string, len(backupDirs.AgentHostsToBackupDir))

		cmd := exectest.NewCommandWithVerifier(hub.Success, func(name string, args ...string) {
			expected := "rsync"
			if !strings.HasSuffix(name, expected) {
				t.Errorf("got %q, want %q", name, expected)
			}

			actualArgsChan <- args
		})
		rsync.SetRsyncCommand(cmd)
		defer rsync.ResetRsyncCommand()

		err := hub.CopyCoordinatorTablespaces(step.DevNullStream, semver.MustParse("5.0.0"), Tablespaces, backupDirs.AgentHostsToBackupDir)
		if err != nil {
			t.Errorf("copying coordinator tablespace directories and mapping file: %+v", err)
		}

		close(actualArgsChan)

		var expectedArgs Args
		for host, backupDir := range backupDirs.AgentHostsToBackupDir {
			expectedArgs = append(expectedArgs, []string{
				"--archive", "--compress", "--delete", "--stats",
				utils.GetStateDirOldTablespacesFile(), "/tmp/tblspc2",
				fmt.Sprintf("%s:%s", host, utils.GetTablespaceBackupDir(backupDir)+string(os.PathSeparator))})
		}

		verifyArgs(t, actualArgsChan, expectedArgs)
	})

	t.Run("when source version is 5X it still copy's the --old-tablespace-file even when there are no user defined coordinator tablespaces", func(t *testing.T) {
		actualArgsChan := make(chan []string, len(backupDirs.AgentHostsToBackupDir))

		cmd := exectest.NewCommandWithVerifier(hub.Success, func(name string, args ...string) {
			expected := "rsync"
			if !strings.HasSuffix(name, expected) {
				t.Errorf("got %q, want %q", name, expected)
			}

			actualArgsChan <- args
		})
		rsync.SetRsyncCommand(cmd)
		defer rsync.ResetRsyncCommand()

		err := hub.CopyCoordinatorTablespaces(step.DevNullStream, semver.MustParse("5.0.0"), nil, backupDirs.AgentHostsToBackupDir)
		if err != nil {
			t.Errorf("got %+v, want nil", err)
		}

		close(actualArgsChan)

		var expectedArgs Args
		for host, backupDir := range backupDirs.AgentHostsToBackupDir {
			expectedArgs = append(expectedArgs, []string{
				"--archive", "--compress", "--delete", "--stats",
				utils.GetStateDirOldTablespacesFile(),
				fmt.Sprintf("%s:%s", host, utils.GetTablespaceBackupDir(backupDir)+string(os.PathSeparator))})
		}

		verifyArgs(t, actualArgsChan, expectedArgs)
	})

	t.Run("when source version is 6X and higher it does not copy the --old-tablespace-file", func(t *testing.T) {
		actualArgsChan := make(chan []string, len(backupDirs.AgentHostsToBackupDir))

		cmd := exectest.NewCommandWithVerifier(hub.Success, func(name string, args ...string) {
			expected := "rsync"
			if !strings.HasSuffix(name, expected) {
				t.Errorf("got %q, want %q", name, expected)
			}

			actualArgsChan <- args
		})
		rsync.SetRsyncCommand(cmd)
		defer rsync.ResetRsyncCommand()

		err := hub.CopyCoordinatorTablespaces(step.DevNullStream, semver.MustParse("6.0.0"), Tablespaces, backupDirs.AgentHostsToBackupDir)
		if err != nil {
			t.Errorf("copying coordinator tablespace directories and mapping file: %+v", err)
		}

		close(actualArgsChan)

		var expectedArgs Args
		for host, backupDir := range backupDirs.AgentHostsToBackupDir {
			expectedArgs = append(expectedArgs, []string{
				"--archive", "--compress", "--delete", "--stats",
				"/tmp/tblspc2",
				fmt.Sprintf("%s:%s", host, utils.GetTablespaceBackupDir(backupDir)+string(os.PathSeparator))})
		}

		verifyArgs(t, actualArgsChan, expectedArgs)
	})

	t.Run("when source version is 6X and there are no tablespaces it does not copy", func(t *testing.T) {
		actualArgsChan := make(chan []string, len(backupDirs.AgentHostsToBackupDir))

		cmd := exectest.NewCommandWithVerifier(hub.Success, func(name string, args ...string) {
			expected := "rsync"
			if !strings.HasSuffix(name, expected) {
				t.Errorf("got %q, want %q", name, expected)
			}

			actualArgsChan <- args
		})
		rsync.SetRsyncCommand(cmd)
		defer rsync.ResetRsyncCommand()

		err := hub.CopyCoordinatorTablespaces(step.DevNullStream, semver.MustParse("6.0.0"), nil, backupDirs.AgentHostsToBackupDir)
		if err != nil {
			t.Errorf("copying coordinator tablespace directories and mapping file: %+v", err)
		}

		close(actualArgsChan)
		var actualArgs Args
		for args := range actualArgsChan {
			actualArgs = append(actualArgs, args)
		}
		sort.Sort(actualArgs)

		if actualArgs != nil {
			t.Errorf("Rsync() should not be invoked")
		}
	})
}

type Args [][]string

func (a Args) Len() int {
	return len(a)
}
func (a Args) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a Args) Less(i, j int) bool {
	hosti := strings.SplitN(a[i][len(a[i])-1], ":", 2)[0]
	hostj := strings.SplitN(a[j][len(a[j])-1], ":", 2)[0]

	return hosti < hostj
}

func verifyArgs(t *testing.T, actualArgsChan chan []string, expectedArgs Args) {
	t.Helper()

	var actualArgs Args
	for args := range actualArgsChan {
		actualArgs = append(actualArgs, args)
	}

	sort.Sort(actualArgs)
	sort.Sort(expectedArgs)

	if !reflect.DeepEqual(actualArgs, expectedArgs) {
		t.Errorf("got %v want %v", actualArgs, expectedArgs)
		t.Errorf("got  %v", actualArgs)
		t.Errorf("want %v", expectedArgs)
	}
}
