// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

func Success() {}
func Failure() {
	os.Exit(1)
}

const FailureStdout = `
Checking for orphaned TOAST relations                       ok
Checking for gphdfs external tables                         ok
Checking for users assigned the gphdfs role                 fatal

| Your installation contains roles that have gphdfs privileges.
| These privileges need to be revoked before upgrade.  A list
| of roles and their corresponding gphdfs privileges that
| must be revoked is provided in the file:
|       gphdfs_user_roles.txt

Failure, exiting
`

const FailureWithTimingStdout = `
Checking for orphaned TOAST relations                       ok [ 1h36m ]
Checking for gphdfs external tables                         ok [ 12s ]
Checking for users assigned the gphdfs role                 fatal [ 36ms ]

| Your installation contains roles that have gphdfs privileges.
| These privileges need to be revoked before upgrade.  A list
| of roles and their corresponding gphdfs privileges that
| must be revoked is provided in the file:
|       gphdfs_user_roles.txt

Failure, exiting
`

func PgCheckFailure() {
	os.Stdout.WriteString(FailureStdout)
	os.Exit(1)
}

func PgCheckFailureWithTiming() {
	os.Stdout.WriteString(FailureWithTimingStdout)
	os.Exit(1)
}

const StreamingMainStdout = "expected\nstdout\n"
const StreamingMainStderr = "process\nstderr\n"

// Streams the above stdout/err constants to the corresponding standard file
// descriptors, alternately interleaving five-byte chunks.
func StreamingMain() {
	stdout := bytes.NewBufferString(StreamingMainStdout)
	stderr := bytes.NewBufferString(StreamingMainStderr)

	for stdout.Len() > 0 || stderr.Len() > 0 {
		os.Stdout.Write(stdout.Next(5))
		os.Stderr.Write(stderr.Next(5))
	}
}

// Writes to stdout and ignores any failure to do so.
func BlindlyWritingMain() {
	// Ignore SIGPIPE. Note that the obvious signal.Ignore(syscall.SIGPIPE)
	// doesn't work as expected; see https://github.com/golang/go/issues/32386.
	signal.Notify(make(chan os.Signal, 1), syscall.SIGPIPE)

	fmt.Println("blah blah blah blah")
	fmt.Println("blah blah blah blah")
	fmt.Println("blah blah blah blah")
}

func init() {
	exectest.RegisterMains(
		Success,
		StreamingMain,
		BlindlyWritingMain,
		Failure,
		PgCheckFailure,
		PgCheckFailureWithTiming,
	)
}

// Writes the current working directory to stdout.
func WorkingDirectoryMain() {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get working directory: %v", err)
		os.Exit(1)
	}

	fmt.Print(wd)
}

// Prints the environment, one variable per line, in NAME=VALUE format.
func EnvironmentMain() {
	for _, e := range os.Environ() {
		fmt.Println(e)
	}
}

func init() {
	exectest.RegisterMains(
		WorkingDirectoryMain,
		EnvironmentMain,
	)
}

func TestUpgradeMaster(t *testing.T) {
	testlog.SetupLogger()

	source := MustCreateCluster(t, []greenplum.SegConfig{
		{ContentID: -1, Port: 5432, DataDir: "/data/old", DbID: 1, Role: "p"},
		{ContentID: -1, Port: 5433, DataDir: "/data/standby", DbID: 2, Role: "m"},
	})
	source.GPHome = "/usr/local/source"

	t.Run("masterSegmentFromCluster() creates a correct upgrade segment", func(t *testing.T) {
		seg := masterSegmentFromCluster(source)

		expected := filepath.Join(source.GPHome, "bin")
		if seg.BinDir != expected {
			t.Errorf("BinDir was %q, want %q", seg.BinDir, expected)
		}
		if seg.DataDir != source.MasterDataDir() {
			t.Errorf("DataDir was %q, want %q", seg.DataDir, source.MasterDataDir())
		}
		if seg.DBID != source.GetDbidForContent(-1) {
			t.Errorf("DBID was %d, want %d", seg.DBID, source.GetDbidForContent(-1))
		}
		if seg.Port != source.MasterPort() {
			t.Errorf("Port was %d, want %d", seg.Port, source.MasterPort())
		}
	})

	// UpgradeMaster defers to upgrade.Run() for most of its work. Rather than
	// repeat those tests here, do some simple integration tests to verify that
	// output streams are hooked up correctly, then defer to the acceptance
	// tests for full end-to-end verification.

	target := MustCreateCluster(t, []greenplum.SegConfig{
		{ContentID: -1, Port: 5433, DataDir: "/data/new", DbID: 2, Role: "p"},
	})
	target.GPHome = "/usr/local/target"
	target.Version = dbconn.NewVersion("6.9.0")

	// We need a real temporary directory to change to. Replace MkdirAll() so
	// that we can make sure the directory is the correct one.
	tempDir, err := ioutil.TempDir("", "gpupgrade")
	if err != nil {
		t.Fatalf("creating temporary directory: %+v", err)
	}
	defer os.RemoveAll(tempDir)

	var createdWD string
	utils.System.MkdirAll = func(path string, perms os.FileMode) error {
		createdWD = path

		// Bail out if the implementation tries to touch any other directories.
		if !strings.HasPrefix(path, tempDir) {
			t.Fatalf("requested directory %q is not under temporary directory %q; refusing to create it",
				path, tempDir)
		}

		return os.MkdirAll(path, perms)
	}
	defer func() {
		utils.System = utils.InitializeSystemFunctions()
	}()

	t.Run("creates the desired working directory", func(t *testing.T) {
		SetExecCommand(exectest.NewCommand(Success))
		defer ResetExecCommand()

		rsync.SetRsyncCommand(exectest.NewCommand(Success))
		defer rsync.ResetRsyncCommand()

		err := UpgradeMaster(UpgradeMasterArgs{
			Source:      source,
			Target:      target,
			StateDir:    tempDir,
			Stream:      step.DevNullStream,
			CheckOnly:   false,
			UseLinkMode: false,
		})
		if err != nil {
			t.Errorf("returned error %+v", err)
		}

		expectedWD := upgrade.MasterWorkingDirectory(tempDir)
		if createdWD != expectedWD {
			t.Errorf("created working directory %q, want %q", createdWD, expectedWD)
		}
	})

	t.Run("sets error text to upgrading when pg_upgrade fails", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(Success))
		defer rsync.ResetRsyncCommand()

		SetExecCommand(exectest.NewCommand(Failure))
		defer ResetExecCommand()

		err := UpgradeMaster(UpgradeMasterArgs{
			Source:      source,
			Target:      target,
			StateDir:    tempDir,
			Stream:      new(step.BufferedStreams),
			CheckOnly:   false,
			UseLinkMode: false,
		})
		if err == nil {
			t.Errorf("expected error, returned nil")
		}

		var upgradeErr UpgradeMasterError
		if !errors.As(err, &upgradeErr) {
			t.Errorf("got type %T want %T", err, upgradeErr)
		}

		if upgradeErr.FailedAction != "upgrade" {
			t.Errorf("got FailedAction %q want upgrade", upgradeErr.FailedAction)
		}
	})

	t.Run("streams stdout and stderr to the client", func(t *testing.T) {
		SetExecCommand(exectest.NewCommand(StreamingMain))
		defer ResetExecCommand()

		rsync.SetRsyncCommand(exectest.NewCommand(Success))
		defer rsync.ResetRsyncCommand()

		stream := new(step.BufferedStreams)

		err := UpgradeMaster(UpgradeMasterArgs{
			Source:      source,
			Target:      target,
			StateDir:    tempDir,
			Stream:      stream,
			CheckOnly:   false,
			UseLinkMode: false,
		})
		if err != nil {
			t.Errorf("returned error %+v", err)
		}

		stdout := stream.StdoutBuf.String()
		if stdout != StreamingMainStdout {
			t.Errorf("got stdout %q, want %q", stdout, StreamingMainStdout)
		}

		stderr := stream.StderrBuf.String()
		if stderr != StreamingMainStderr {
			t.Errorf("got stderr %q, want %q", stderr, StreamingMainStderr)
		}
	})

	t.Run("sets the standby dbid on the master if the GPDB version is 5", func(t *testing.T) {
		execCmd := exectest.NewCommandWithVerifier(Success, func(command string, args ...string) {
			expected := "--old-options -x 2"
			if !strings.Contains(strings.Join(args, " "), expected) {
				t.Errorf("did not find %q in the args %q", expected, args)
			}
		})
		SetExecCommand(execCmd)
		defer ResetExecCommand()

		rsync.SetRsyncCommand(exectest.NewCommand(Success))
		defer rsync.ResetRsyncCommand()

		source.Version = dbconn.NewVersion("5.28.0")

		err := UpgradeMaster(UpgradeMasterArgs{
			Source:      source,
			Target:      target,
			StateDir:    tempDir,
			Stream:      step.DevNullStream,
			CheckOnly:   false,
			UseLinkMode: false,
		})
		if err != nil {
			t.Errorf("returned error %+v", err)
		}
	})

	t.Run("does not set the standby dbid on the master if the GPDB version is 6", func(t *testing.T) {
		execCmd := exectest.NewCommandWithVerifier(Success, func(command string, args ...string) {
			for _, arg := range args {
				if arg == "--old-options" {
					t.Errorf("expected --old-options to not be in args %q", args)
				}
			}
		})
		SetExecCommand(execCmd)
		defer ResetExecCommand()

		rsync.SetRsyncCommand(exectest.NewCommand(Success))
		defer rsync.ResetRsyncCommand()

		source.Version = dbconn.NewVersion("6.10.0")

		err := UpgradeMaster(UpgradeMasterArgs{
			Source:      source,
			Target:      target,
			StateDir:    tempDir,
			Stream:      step.DevNullStream,
			CheckOnly:   false,
			UseLinkMode: false,
		})
		if err != nil {
			t.Errorf("returned error %+v", err)
		}
	})

	t.Run("returns an error if the command succeeds but the io.Writer fails", func(t *testing.T) {
		// Don't fail in the subprocess even when the stdout stream is closed.
		SetExecCommand(exectest.NewCommand(BlindlyWritingMain))
		defer ResetExecCommand()

		rsync.SetRsyncCommand(exectest.NewCommand(Success))
		defer rsync.ResetRsyncCommand()

		expectedErr := errors.New("write failed!")
		err := UpgradeMaster(UpgradeMasterArgs{
			Source:      source,
			Target:      target,
			StateDir:    tempDir,
			Stream:      testutils.FailingStreams{Err: expectedErr},
			CheckOnly:   false,
			UseLinkMode: false,
		})
		if !errors.Is(err, expectedErr) {
			t.Errorf("returned error %+v, want %+v", err, expectedErr)
		}
	})

	t.Run("rsync during upgrade master errors out", func(t *testing.T) {
		SetExecCommand(exectest.NewCommand(StreamingMain))
		defer ResetExecCommand()

		rsync.SetRsyncCommand(exectest.NewCommand(Failure))
		defer rsync.ResetRsyncCommand()

		stream := new(step.BufferedStreams)

		err := UpgradeMaster(UpgradeMasterArgs{
			Source:      source,
			Target:      target,
			StateDir:    tempDir,
			Stream:      stream,
			CheckOnly:   false,
			UseLinkMode: false,
		})
		if err == nil {
			t.Errorf("expected error, returned nil")
		}

	})

	t.Run("when pg_upgrade check fails it adds stdout context to the error", func(t *testing.T) {
		cases := []struct {
			name     string
			main     exectest.Main
			expected string
		}{
			{
				"without timing", PgCheckFailure, strings.TrimSpace(`
Checking for users assigned the gphdfs role                 fatal

| Your installation contains roles that have gphdfs privileges.
| These privileges need to be revoked before upgrade.  A list
| of roles and their corresponding gphdfs privileges that
| must be revoked is provided in the file:
|       %s/gphdfs_user_roles.txt

Failure, exiting
				`),
			}, {
				"with timing", PgCheckFailureWithTiming, strings.TrimSpace(`
Checking for users assigned the gphdfs role                 fatal [ 36ms ]

| Your installation contains roles that have gphdfs privileges.
| These privileges need to be revoked before upgrade.  A list
| of roles and their corresponding gphdfs privileges that
| must be revoked is provided in the file:
|       %s/gphdfs_user_roles.txt

Failure, exiting
				`),
			},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				cmd, mock := exectest.NewCommandMock(ctrl)
				SetExecCommand(cmd)
				defer ResetExecCommand()

				rsync.SetRsyncCommand(exectest.NewCommand(Success))
				defer rsync.ResetRsyncCommand()

				stream := new(step.BufferedStreams)

				// create a file in the working directory of upgrade_master just like pg_upgrade would do
				mock.EXPECT().Command("/usr/local/target/bin/pg_upgrade", gomock.Any()).
					DoAndReturn(func(string, ...string) exectest.Main {
						testutils.MustWriteToFile(t, filepath.Join(createdWD, "gphdfs_user_roles.txt"), "")
						return c.main
					})

				err := UpgradeMaster(UpgradeMasterArgs{
					Source:      source,
					Target:      target,
					StateDir:    tempDir,
					Stream:      stream,
					CheckOnly:   true,
					UseLinkMode: false,
				})
				if err == nil {
					t.Errorf("expected error, returned nil")
				}

				var upgradeErr UpgradeMasterError
				if !errors.As(err, &upgradeErr) {
					t.Errorf("got type %T want %T", err, upgradeErr)
				}

				if upgradeErr.FailedAction != "check" {
					t.Errorf("got FailedAction %q want check", upgradeErr.FailedAction)
				}

				expected := fmt.Sprintf(c.expected, createdWD)
				if upgradeErr.ErrorText != expected {
					t.Errorf("actual error text does not match expected")
					t.Logf("got:\n%s", upgradeErr.ErrorText)
					t.Logf("want:\n%s", expected)
				}
			})
		}
	})
}

func TestRsyncMasterDir(t *testing.T) {
	t.Run("rsync streams stdout and stderr to the client", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(StreamingMain))
		defer rsync.ResetRsyncCommand()

		stream := new(step.BufferedStreams)
		err := RsyncMasterDataDir(stream, "", "")

		if err != nil {
			t.Errorf("returned: %+v", err)
		}

		stdout := stream.StdoutBuf.String()
		if stdout != StreamingMainStdout {
			t.Errorf("got stdout %q, want %q", stdout, StreamingMainStdout)
		}

		stderr := stream.StderrBuf.String()
		if stderr != StreamingMainStderr {
			t.Errorf("got stderr %q, want %q", stderr, StreamingMainStderr)
		}
	})

}
