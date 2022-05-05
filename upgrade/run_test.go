// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package upgrade_test

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"reflect"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

// Prints the strings "stdout" and "stderr" to the respective streams.
func PrintMain() {
	fmt.Fprint(os.Stdout, "stdout")
	fmt.Fprint(os.Stderr, "stderr")
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
		PrintMain,
		WorkingDirectoryMain,
		EnvironmentMain,
	)
}

func TestRun(t *testing.T) {
	testlog.SetupLogger()

	t.Run("creates the pg_upgrade working directory", func(t *testing.T) {
		var called bool
		utils.System.MkdirAll = func(path string, perms os.FileMode) error {
			called = true

			expected, err := utils.GetPgUpgradeDir(greenplum.MirrorRole, 3)
			if err != nil {
				t.Fatal(err)
			}

			if path != expected {
				t.Fatalf("got pg_upgrade working directory %q want %q", path, expected)
			}

			testutils.MustRemoveAll(t, path)
			return os.MkdirAll(path, perms)
		}
		defer utils.ResetSystemFunctions()

		upgrade.SetPgUpgradeCommand(exectest.NewCommand(upgrade.Success))
		defer upgrade.ResetPgUpgradeCommand()

		opts := &idl.PgOptions{
			Role:          greenplum.MirrorRole,
			ContentID:     3,
			TargetVersion: "6.20.0",
		}

		err := upgrade.Run(nil, nil, opts)
		if err != nil {
			t.Fatalf("unexpected error %+v", err)
		}

		if !called {
			t.Errorf("expected mkdir to be called for pg_upgrade directory")
		}
	})

	t.Run("does not fail if the pg_upgrade working directory already exists", func(t *testing.T) {
		expected, err := utils.GetPgUpgradeDir(greenplum.MirrorRole, 3)
		if err != nil {
			t.Fatal(err)
		}

		testutils.MustCreateDir(t, expected)

		upgrade.SetPgUpgradeCommand(exectest.NewCommand(upgrade.Success))
		defer upgrade.ResetPgUpgradeCommand()

		opts := &idl.PgOptions{
			Role:          greenplum.MirrorRole,
			ContentID:     3,
			TargetVersion: "6.20.0",
		}

		err = upgrade.Run(nil, nil, opts)
		if err != nil {
			t.Fatalf("unexpected error %+v", err)
		}
	})

	t.Run("errors when getting the pg_upgrade directory fails", func(t *testing.T) {
		expected := os.ErrPermission
		utils.System.Current = func() (*user.User, error) {
			return nil, expected
		}
		defer utils.ResetSystemFunctions()

		upgrade.SetPgUpgradeCommand(exectest.NewCommand(upgrade.Success))
		defer upgrade.ResetPgUpgradeCommand()

		err := upgrade.Run(nil, nil, &idl.PgOptions{})
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v want %#v", err, expected)
		}
	})

	t.Run("errors when creating the pg_upgrade working directory fails", func(t *testing.T) {
		expected := os.ErrPermission
		utils.System.MkdirAll = func(path string, perms os.FileMode) error {
			return expected
		}
		defer utils.ResetSystemFunctions()

		upgrade.SetPgUpgradeCommand(exectest.NewCommand(upgrade.Success))
		defer upgrade.ResetPgUpgradeCommand()

		err := upgrade.Run(nil, nil, &idl.PgOptions{})
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v want %#v", err, expected)
		}
	})

	t.Run("can control output destinations", func(t *testing.T) {
		upgrade.SetPgUpgradeCommand(exectest.NewCommand(PrintMain))
		defer upgrade.ResetPgUpgradeCommand()

		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)

		opts := &idl.PgOptions{
			Role:          greenplum.MirrorRole,
			ContentID:     3,
			TargetVersion: "6.20.0",
		}
		err := upgrade.Run(stdout, stderr, opts)
		if err != nil {
			t.Fatalf("unexpected error %+v", err)
		}

		actual := stdout.String()
		if actual != "stdout" {
			t.Errorf("stdout contents were %q, want %q", actual, "stdout")
		}

		actual = stderr.String()
		if actual != "stderr" {
			t.Errorf("stderr contents were %q, want %q", actual, "stderr")
		}
	})

	t.Run("can set the working directory", func(t *testing.T) {
		// Print the working directory of the command.
		upgrade.SetPgUpgradeCommand(exectest.NewCommand(WorkingDirectoryMain))
		defer upgrade.ResetPgUpgradeCommand()

		stdout := new(bytes.Buffer)

		opts := &idl.PgOptions{
			Role:          greenplum.MirrorRole,
			ContentID:     3,
			TargetVersion: "6.20.0",
		}
		err := upgrade.Run(stdout, nil, opts)
		if err != nil {
			t.Fatalf("unexpected error %+v", err)
		}

		expected, err := utils.GetPgUpgradeDir(greenplum.MirrorRole, 3)
		if err != nil {
			t.Fatal(err)
		}

		actual := stdout.String()
		if actual != expected {
			t.Errorf("working directory was %q, want %q", actual, expected)
		}
	})

	t.Run("unsets PGPORT and PGHOST", func(t *testing.T) {
		// Set our environment.
		resetPort := testutils.SetEnv(t, "PGPORT", "5432")
		defer resetPort()

		resetHost := testutils.SetEnv(t, "PGHOST", "localhost")
		defer resetHost()

		// Echo the environment to stdout.
		upgrade.SetPgUpgradeCommand(exectest.NewCommand(EnvironmentMain))
		defer upgrade.ResetPgUpgradeCommand()

		stdout := new(bytes.Buffer)

		opts := &idl.PgOptions{
			Role:          greenplum.MirrorRole,
			ContentID:     3,
			TargetVersion: "6.20.0",
		}
		err := upgrade.Run(stdout, nil, opts)
		if err != nil {
			t.Fatalf("unexpected error %+v", err)
		}

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()

			if strings.HasPrefix(line, "PGPORT=") {
				t.Error("PGPORT was not stripped from the child environment")
			}
			if strings.HasPrefix(line, "PGHOST=") {
				t.Error("PGHOST was not stripped from the child environment")
			}
		}
		if err := scanner.Err(); err != nil {
			t.Errorf("got error during scan: %+v", err)
		}
	})

	t.Run("when run fails it returns an error", func(t *testing.T) {
		upgrade.SetPgUpgradeCommand(exectest.NewCommand(upgrade.Failure))
		defer upgrade.ResetPgUpgradeCommand()

		opts := &idl.PgOptions{
			Role:          greenplum.MirrorRole,
			ContentID:     3,
			TargetVersion: "6.20.0",
		}

		err := upgrade.Run(nil, nil, opts)
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("got error %#v, want type *exec.ExitError", err)
		}

		if exitErr.ExitCode() != 1 {
			t.Errorf("got exit code %d, want 1", exitErr.ExitCode())
		}
	})

	cases := []struct {
		name         string
		expectedCmd  string
		expectedArgs []string
		opts         idl.PgOptions
	}{
		{
			name:        "run uses correct arguments based on pg options",
			expectedCmd: "/usr/local/new/bin/dir/pg_upgrade",
			expectedArgs: []string{"--retain", "--progress",
				"--old-bindir", "/usr/local/old/bin/dir",
				"--new-bindir", "/usr/local/new/bin/dir",
				"--old-datadir", "/old/data/dir",
				"--new-datadir", "/new/data/dir",
				"--old-port", "1234",
				"--new-port", "7890",
				"--mode", "dispatcher",
				"--check", "--link",
				"--old-options", "-x 2",
				"--old-gp-dbid", "88",
				"--new-gp-dbid", "99"},
			opts: idl.PgOptions{
				Role:          greenplum.PrimaryRole,
				ContentID:     3,
				Mode:          idl.PgOptions_Dispatcher,
				OldOptions:    "-x 2",
				Action:        idl.PgOptions_check,
				LinkMode:      true,
				TargetVersion: "6.20.0",
				OldBinDir:     "/usr/local/old/bin/dir",
				OldDataDir:    "/old/data/dir",
				OldPort:       "1234",
				OldDBID:       "88",
				NewBinDir:     "/usr/local/new/bin/dir",
				NewDataDir:    "/new/data/dir",
				NewPort:       "7890",
				NewDBID:       "99",
				Tablespaces: map[int32]*idl.TablespaceInfo{
					1663: {Name: "tblspc1", Location: "/tmp/primary1/1663", UserDefined: false},
					1664: {Name: "tblspc2", Location: "/tmp/primary1/1664", UserDefined: true}},
			},
		},
		{
			name: "sets --check when Check	 is true",
			expectedCmd: "pg_upgrade",
			expectedArgs: []string{"--retain", "--progress",
				"--old-bindir", "",
				"--new-bindir", "",
				"--old-datadir", "",
				"--new-datadir", "",
				"--old-port", "",
				"--new-port", "",
				"--mode", "unknown_mode",
				"--check",
				"--old-gp-dbid", "",
				"--new-gp-dbid", ""},
			opts: idl.PgOptions{
				Role:          greenplum.PrimaryRole,
				ContentID:     3,
				Action:        idl.PgOptions_check,
				TargetVersion: "6.20.0",
			},
		},
		{
			name: "does not set --check when Check	 is false",
			expectedCmd: "pg_upgrade",
			expectedArgs: []string{"--retain", "--progress",
				"--old-bindir", "",
				"--new-bindir", "",
				"--old-datadir", "",
				"--new-datadir", "",
				"--old-port", "",
				"--new-port", "",
				"--mode", "unknown_mode",
				"--old-tablespaces-file", utils.GetTablespaceMappingFile(),
				"--old-gp-dbid", "",
				"--new-gp-dbid", ""},
			opts: idl.PgOptions{
				Role:          greenplum.PrimaryRole,
				ContentID:     3,
				Action:        idl.PgOptions_upgrade,
				TargetVersion: "6.20.0",
			},
		},
		{
			name:        "sets --link when LinkMode is true",
			expectedCmd: "pg_upgrade",
			expectedArgs: []string{"--retain", "--progress",
				"--old-bindir", "",
				"--new-bindir", "",
				"--old-datadir", "",
				"--new-datadir", "",
				"--old-port", "",
				"--new-port", "",
				"--mode", "unknown_mode",
				"--link",
				"--old-tablespaces-file", utils.GetTablespaceMappingFile(),
				"--old-gp-dbid", "",
				"--new-gp-dbid", ""},
			opts: idl.PgOptions{
				Role:          greenplum.PrimaryRole,
				ContentID:     3,
				LinkMode:      true,
				TargetVersion: "6.20.0",
			},
		},
		{
			name:        "does not set --link when LinkMode is true",
			expectedCmd: "pg_upgrade",
			expectedArgs: []string{"--retain", "--progress",
				"--old-bindir", "",
				"--new-bindir", "",
				"--old-datadir", "",
				"--new-datadir", "",
				"--old-port", "",
				"--new-port", "",
				"--mode", "unknown_mode",
				"--old-tablespaces-file", utils.GetTablespaceMappingFile(),
				"--old-gp-dbid", "",
				"--new-gp-dbid", ""},
			opts: idl.PgOptions{
				Role:          greenplum.PrimaryRole,
				ContentID:     3,
				LinkMode:      false,
				TargetVersion: "6.20.0",
			},
		},
		{
			name:        "does not set --old-tablespaces-file when --check is passed",
			expectedCmd: "pg_upgrade",
			expectedArgs: []string{"--retain", "--progress",
				"--old-bindir", "",
				"--new-bindir", "",
				"--old-datadir", "",
				"--new-datadir", "",
				"--old-port", "",
				"--new-port", "",
				"--mode", "unknown_mode",
				"--check",
				"--old-gp-dbid", "",
				"--new-gp-dbid", ""},
			opts: idl.PgOptions{
				Role:          greenplum.PrimaryRole,
				ContentID:     -1,
				Action:        idl.PgOptions_check,
				TargetVersion: "6.20.0",
			},
		},
		{
			name:        "sets --old-tablespaces-file when upgrading and not calling --check",
			expectedCmd: "pg_upgrade",
			expectedArgs: []string{"--retain", "--progress",
				"--old-bindir", "",
				"--new-bindir", "",
				"--old-datadir", "",
				"--new-datadir", "",
				"--old-port", "",
				"--new-port", "",
				"--mode", "unknown_mode",
				"--old-tablespaces-file", utils.GetTablespaceMappingFile(),
				"--old-gp-dbid", "",
				"--new-gp-dbid", ""},
			opts: idl.PgOptions{
				Role:          greenplum.PrimaryRole,
				ContentID:     3,
				Action:        idl.PgOptions_upgrade,
				TargetVersion: "6.20.0",
			},
		},
		{
			name:        "sets --old-gp-dbid and --new-gp-dbid when target version is less than 7X",
			expectedCmd: "pg_upgrade",
			expectedArgs: []string{"--retain", "--progress",
				"--old-bindir", "",
				"--new-bindir", "",
				"--old-datadir", "",
				"--new-datadir", "",
				"--old-port", "",
				"--new-port", "",
				"--mode", "unknown_mode",
				"--old-tablespaces-file", utils.GetTablespaceMappingFile(),
				"--old-gp-dbid", "0",
				"--new-gp-dbid", "1"},
			opts: idl.PgOptions{
				Role:          greenplum.PrimaryRole,
				ContentID:     3,
				LinkMode:      false,
				TargetVersion: "6.20.0",
				OldDBID:       "0",
				NewDBID:       "1",
			},
		},
		{
			name:        "does not set --old-gp-dbid and --new-gp-dbid when target version 7X or higher",
			expectedCmd: "pg_upgrade",
			expectedArgs: []string{"--retain", "--progress",
				"--old-bindir", "",
				"--new-bindir", "",
				"--old-datadir", "",
				"--new-datadir", "",
				"--old-port", "",
				"--new-port", "",
				"--mode", "unknown_mode",
				"--old-tablespaces-file", utils.GetTablespaceMappingFile()},
			opts: idl.PgOptions{
				Role:          greenplum.PrimaryRole,
				ContentID:     3,
				LinkMode:      false,
				TargetVersion: "7.1.0",
				OldDBID:       "0",
				NewDBID:       "1",
			},
		},
		{
			name:        "does not set --old-options when they are not specified",
			expectedCmd: "pg_upgrade",
			expectedArgs: []string{"--retain", "--progress",
				"--old-bindir", "",
				"--new-bindir", "",
				"--old-datadir", "",
				"--new-datadir", "",
				"--old-port", "",
				"--new-port", "",
				"--mode", "unknown_mode",
				"--old-tablespaces-file", utils.GetTablespaceMappingFile(),
				"--old-gp-dbid", "",
				"--new-gp-dbid", ""},
			opts: idl.PgOptions{
				Role:          greenplum.PrimaryRole,
				ContentID:     3,
				TargetVersion: "6.20.0",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			upgrade.SetPgUpgradeCommand(exectest.NewCommandWithVerifier(upgrade.Success, func(command string, args ...string) {
				if command != c.expectedCmd {
					t.Errorf("got %q want %q", command, c.expectedCmd)
				}

				if !reflect.DeepEqual(args, c.expectedArgs) {
					t.Errorf("expected args do not match")
					t.Errorf("got  %q", args)
					t.Errorf("want %q", c.expectedArgs)
				}
			}))
			defer upgrade.ResetPgUpgradeCommand()

			err := upgrade.Run(nil, nil, &c.opts)
			if err != nil {
				t.Fatalf("unexpected error %+v", err)
			}
		})
	}
}
