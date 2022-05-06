// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

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
		BlindlyWritingMain,
		PgCheckFailure,
		PgCheckFailureWithTiming,
	)
}

func TestUpgradeMaster(t *testing.T) {
	testlog.SetupLogger()

	pgUpgradeDir, err := utils.GetPgUpgradeDir(greenplum.PrimaryRole, -1)
	if err != nil {
		t.Fatal(err)
	}

	source := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, Port: 5432, DataDir: "/data/old", DbID: 1, Role: greenplum.PrimaryRole},
		{ContentID: -1, Port: 5433, DataDir: "/data/standby", DbID: 2, Role: greenplum.MirrorRole},
	})
	source.GPHome = "/usr/local/source"

	intermediate := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, Port: 5433, DataDir: "/data/new", DbID: 2, Role: greenplum.PrimaryRole},
	})
	intermediate.GPHome = "/usr/local/target"
	intermediate.Version = semver.MustParse("6.15.0")

	t.Run("succeeds", func(t *testing.T) {
		upgrade.SetPgUpgradeCommand(exectest.NewCommand(hub.StreamingMain))
		defer upgrade.ResetPgUpgradeCommand()

		rsync.SetRsyncCommand(exectest.NewCommand(hub.Success))
		defer rsync.ResetRsyncCommand()

		streams := new(step.BufferedStreams)
		err := hub.UpgradeMaster(streams, source, intermediate, idl.PgOptions_check, false)
		if err != nil {
			t.Fatalf("unexpected error %+v", err)
		}

		stdout := streams.StdoutBuf.String()
		if stdout != hub.StreamingMainStdout {
			t.Errorf("got stdout %q, want %q", stdout, hub.StreamingMainStdout)
		}

		stderr := streams.StderrBuf.String()
		if stderr != hub.StreamingMainStderr {
			t.Errorf("got stderr %q, want %q", stderr, hub.StreamingMainStderr)
		}
	})

	t.Run("sets the old options when upgrading from GPDB 5 with a standby", func(t *testing.T) {
		upgrade.SetPgUpgradeCommand(exectest.NewCommandWithVerifier(hub.Success, func(command string, args ...string) {
			expected := "--old-options -x 2"
			if !strings.Contains(strings.Join(args, " "), expected) {
				t.Errorf("did not find %q in the args %q", expected, args)
			}
		}))
		defer upgrade.ResetPgUpgradeCommand()

		rsync.SetRsyncCommand(exectest.NewCommand(hub.Success))
		defer rsync.ResetRsyncCommand()

		source.Version = semver.MustParse("5.28.0")

		err := hub.UpgradeMaster(step.DevNullStream, source, intermediate, idl.PgOptions_check, false)
		if err != nil {
			t.Fatalf("unexpected error %+v", err)
		}
	})

	t.Run("does not set the old options when upgrading from GPDB 6 or later", func(t *testing.T) {
		upgrade.SetPgUpgradeCommand(exectest.NewCommandWithVerifier(hub.Success, func(command string, args ...string) {
			for _, arg := range args {
				if arg == "--old-options" {
					t.Errorf("expected --old-options to not be in args %q", args)
				}
			}
		}))
		defer upgrade.ResetPgUpgradeCommand()

		rsync.SetRsyncCommand(exectest.NewCommand(hub.Success))
		defer rsync.ResetRsyncCommand()

		source.Version = semver.MustParse("6.10.0")

		err := hub.UpgradeMaster(step.DevNullStream, source, intermediate, idl.PgOptions_check, false)
		if err != nil {
			t.Fatalf("unexpected error %+v", err)
		}
	})

	t.Run("restores the backup master data directory", func(t *testing.T) {
		upgrade.SetPgUpgradeCommand(exectest.NewCommand(hub.Success))
		defer upgrade.ResetPgUpgradeCommand()

		var called bool
		rsync.SetRsyncCommand(exectest.NewCommandWithVerifier(hub.Success, func(utility string, args ...string) {
			called = true
		}))
		defer rsync.ResetRsyncCommand()

		err := hub.UpgradeMaster(step.DevNullStream, source, intermediate, idl.PgOptions_check, false)
		if err != nil {
			t.Fatalf("unexpected error %+v", err)
		}

		if !called {
			t.Error("expected rsync to be called")
		}
	})

	t.Run("errors when restoring the backup master data directory fails", func(t *testing.T) {
		upgrade.SetPgUpgradeCommand(exectest.NewCommand(hub.Success))
		defer upgrade.ResetPgUpgradeCommand()

		rsync.SetRsyncCommand(exectest.NewCommand(hub.Failure))
		defer rsync.ResetRsyncCommand()

		err := hub.UpgradeMaster(step.DevNullStream, source, intermediate, idl.PgOptions_upgrade, false)
		var actual *exec.ExitError
		if !errors.As(err, &actual) {
			t.Fatalf("got %#v want ExitError", err)
		}

		if actual.ExitCode() != 1 {
			t.Errorf("got %d want 1", actual.ExitCode())
		}

		expected := fmt.Sprintf("rsync %q to %q: exit status 1", utils.GetCoordinatorPreUpgradeBackupDir()+string(os.PathSeparator), intermediate.MasterDataDir())
		if err.Error() != expected {
			t.Errorf("got error message %q want %q", err.Error(), expected)
		}
	})

	t.Run("only when upgrading and not when running --check it restores the backup master data directory", func(t *testing.T) {
		upgrade.SetPgUpgradeCommand(exectest.NewCommand(hub.Success))
		defer upgrade.ResetPgUpgradeCommand()

		rsync.SetRsyncCommand(exectest.NewCommandWithVerifier(hub.Success, func(utility string, args ...string) {
			if !strings.HasSuffix(utility, "rsync") {
				t.Errorf("got %q want rsync", utility)
			}

			options := args[:2]
			expectedOpts := []string{"--archive", "--delete"}
			if !reflect.DeepEqual(options, expectedOpts) {
				t.Errorf("got options %q want %q", options, expectedOpts)
			}

			src := args[2]
			expected := utils.GetCoordinatorPreUpgradeBackupDir() + string(os.PathSeparator)
			if src != expected {
				t.Errorf("got source %q want %q", src, expected)
			}

			dst := args[3]
			expected = intermediate.MasterDataDir()
			if dst != expected {
				t.Errorf("got destination %q want %q", dst, expected)
			}

			exclusions := strings.Join(args[4:], " ")
			expected = "--exclude pg_log/*"
			if !reflect.DeepEqual(exclusions, expected) {
				t.Errorf("got exclusions %q want %q", exclusions, expected)
			}
		}))
		defer rsync.ResetRsyncCommand()

		err := hub.UpgradeMaster(step.DevNullStream, source, intermediate, idl.PgOptions_upgrade, false)
		if err != nil {
			t.Fatalf("unexpected error %+v", err)
		}
	})

	t.Run("errors when pg_upgrade fails and there is no error text", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(hub.Success))
		defer rsync.ResetRsyncCommand()

		upgrade.SetPgUpgradeCommand(exectest.NewCommand(hub.Failure))
		defer upgrade.ResetPgUpgradeCommand()

		err := hub.UpgradeMaster(new(step.BufferedStreams), source, intermediate, idl.PgOptions_upgrade, false)
		expected := "upgrade master: exit status 1"
		if err.Error() != expected {
			t.Errorf("got %q want %q", err.Error(), expected)
		}
	})

	t.Run("returns next actions error when pg_upgrade check fails with context", func(t *testing.T) {
		utils.System.MkdirAll = func(path string, perms os.FileMode) error {
			if path != pgUpgradeDir {
				t.Fatalf("got pg_upgrade working directory %q want %q", path, pgUpgradeDir)
			}

			testutils.MustRemoveAll(t, pgUpgradeDir)
			err := os.MkdirAll(path, perms)
			if err != nil {
				return err
			}

			testutils.MustWriteToFile(t, filepath.Join(pgUpgradeDir, "gphdfs_user_roles.txt"), "")
			return nil
		}
		defer utils.ResetSystemFunctions()

		rsync.SetRsyncCommand(exectest.NewCommand(hub.Success))
		defer rsync.ResetRsyncCommand()

		upgrade.SetPgUpgradeCommand(exectest.NewCommand(PgCheckFailure))
		defer upgrade.ResetPgUpgradeCommand()

		err := hub.UpgradeMaster(new(step.BufferedStreams), source, intermediate, idl.PgOptions_check, false)
		var nextActionsErr utils.NextActionErr
		if !errors.As(err, &nextActionsErr) {
			t.Fatalf("got type %T want %T", err, nextActionsErr)
		}

		errText := fmt.Sprintf(`Checking for users assigned the gphdfs role                 fatal

| Your installation contains roles that have gphdfs privileges.
| These privileges need to be revoked before upgrade.  A list
| of roles and their corresponding gphdfs privileges that
| must be revoked is provided in the file:
|       %s/gphdfs_user_roles.txt

Failure, exiting`, pgUpgradeDir)
		expected := fmt.Sprintf("check master: exit status 1\n\n%s", errText)
		if err.Error() != expected {
			t.Errorf("got  %q", err.Error())
			t.Errorf("want %q", expected)

		}
	})

	t.Run("returns an error if the command succeeds but the io.Writer fails", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(hub.Success))
		defer rsync.ResetRsyncCommand()

		// Don't fail in the subprocess even when the stdout stream is closed.
		upgrade.SetPgUpgradeCommand(exectest.NewCommand(BlindlyWritingMain))
		defer upgrade.ResetPgUpgradeCommand()

		err := hub.UpgradeMaster(testutils.FailingStreams{Err: errors.New("write failed")}, source, intermediate, idl.PgOptions_upgrade, false)
		expected := "upgrade master: write failed"
		if err.Error() != expected {
			t.Errorf("got %q want %q", err.Error(), expected)
		}
	})

	t.Run("when pg_upgrade check fails it adds stdout context to the error", func(t *testing.T) {
		cases := []struct {
			name     string
			main     exectest.Main
			expected string
		}{
			{
				"without timing", PgCheckFailure, fmt.Sprintf(strings.TrimSpace(`
Checking for users assigned the gphdfs role                 fatal

| Your installation contains roles that have gphdfs privileges.
| These privileges need to be revoked before upgrade.  A list
| of roles and their corresponding gphdfs privileges that
| must be revoked is provided in the file:
|       %s/gphdfs_user_roles.txt

Failure, exiting
				`), pgUpgradeDir),
			}, {
				"with timing", PgCheckFailureWithTiming, fmt.Sprintf(strings.TrimSpace(`
Checking for users assigned the gphdfs role                 fatal [ 36ms ]

| Your installation contains roles that have gphdfs privileges.
| These privileges need to be revoked before upgrade.  A list
| of roles and their corresponding gphdfs privileges that
| must be revoked is provided in the file:
|       %s/gphdfs_user_roles.txt

Failure, exiting
				`), pgUpgradeDir),
			},
		}

		utils.System.MkdirAll = func(path string, perms os.FileMode) error {
			if path != pgUpgradeDir {
				t.Fatalf("got pg_upgrade working directory %q want %q", path, pgUpgradeDir)
			}

			testutils.MustRemoveAll(t, pgUpgradeDir)
			err := os.MkdirAll(path, perms)
			if err != nil {
				return err
			}

			testutils.MustWriteToFile(t, filepath.Join(pgUpgradeDir, "gphdfs_user_roles.txt"), "")
			return nil
		}
		defer utils.ResetSystemFunctions()

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				rsync.SetRsyncCommand(exectest.NewCommand(hub.Success))
				defer rsync.ResetRsyncCommand()

				upgrade.SetPgUpgradeCommand(exectest.NewCommand(c.main))
				defer upgrade.ResetPgUpgradeCommand()

				err := hub.UpgradeMaster(new(step.BufferedStreams), source, intermediate, idl.PgOptions_check, false)
				if err == nil {
					t.Errorf("expected error, returned nil")
				}

				var nextActionsErr utils.NextActionErr
				if !errors.As(err, &nextActionsErr) {
					t.Fatalf("got type %T want %T", err, nextActionsErr)
				}

				expected := fmt.Sprintf("check master: exit status 1\n\n%s", c.expected)
				if err.Error() != expected {
					t.Errorf("got  %q", err.Error())
					t.Errorf("want %q", expected)

				}
			})
		}
	})
}

func TestRsyncMasterDir(t *testing.T) {
	t.Run("rsync streams stdout and stderr to the client", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(hub.StreamingMain))
		defer rsync.ResetRsyncCommand()

		stream := new(step.BufferedStreams)
		err := hub.RsyncMasterDataDir(stream, "", "")

		if err != nil {
			t.Errorf("returned: %+v", err)
		}

		stdout := stream.StdoutBuf.String()
		if stdout != hub.StreamingMainStdout {
			t.Errorf("got stdout %q, want %q", stdout, hub.StreamingMainStdout)
		}

		stderr := stream.StderrBuf.String()
		if stderr != hub.StreamingMainStderr {
			t.Errorf("got stderr %q, want %q", stderr, hub.StreamingMainStderr)
		}
	})
}
