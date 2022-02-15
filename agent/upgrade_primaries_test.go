// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

func TestUpgradePrimaries(t *testing.T) {
	testhelper.SetupTestLogger()
	server := agent.NewServer(agent.Config{})

	t.Run("succeeds", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(agent.Success))
		defer rsync.ResetRsyncCommand()

		upgrade.SetPgUpgradeCommand(exectest.NewCommand(agent.Success))
		defer upgrade.ResetPgUpgradeCommand()

		opts := []*idl.PgOptions{{
			Role:          greenplum.PrimaryRole,
			Action:        idl.PgOptions_check,
			TargetVersion: "6.0.0",
		}}

		_, err := server.UpgradePrimaries(context.Background(), &idl.UpgradePrimariesRequest{Opts: opts})
		if err != nil {
			t.Fatalf("unexpected error %+v", err)
		}
	})

	t.Run("restores backup and tablespaces when not calling --check", func(t *testing.T) {
		var calls int
		rsync.SetRsyncCommand(exectest.NewCommandWithVerifier(agent.Success, func(utility string, args ...string) {
			if !strings.HasSuffix(utility, "rsync") {
				t.Errorf("got %q want rsync", utility)
			}

			calls++
		}))

		var symlinks Symlinks
		utils.System.Symlink = func(oldname, newname string) error {
			symlinks = append(symlinks, Symlink{Oldname: oldname, Newname: newname})
			return nil
		}
		defer utils.ResetSystemFunctions()

		upgrade.SetPgUpgradeCommand(exectest.NewCommand(agent.Success))
		defer upgrade.ResetPgUpgradeCommand()

		opts := []*idl.PgOptions{
			{
				Role:          greenplum.PrimaryRole,
				Action:        idl.PgOptions_upgrade,
				TargetVersion: "6.0.0",
				OldDBID:       "1",
				NewDataDir:    "/new/data/dir",
				Tablespaces: map[int32]*idl.TablespaceInfo{
					1663: {Name: "tblspc1", Location: "/tmp/primary1/1663", UserDefined: true},
					1664: {Name: "tblspc2", Location: "/tmp/primary1/1664", UserDefined: true},
				},
			},
		}

		_, err := server.UpgradePrimaries(context.Background(), &idl.UpgradePrimariesRequest{Opts: opts})
		if err != nil {
			t.Fatalf("unexpected error %+v", err)
		}

		if calls != 3 {
			t.Errorf("got %d want 3 calls to rsync. 1 to restore backup and 2 to restore tablespaces", calls)
		}

		expectedSymlinks := Symlinks{
			{Oldname: "/tmp/primary1/1663/1", Newname: "/new/data/dir/pg_tblspc/1663"},
			{Oldname: "/tmp/primary1/1664/1", Newname: "/new/data/dir/pg_tblspc/1664"},
		}

		sort.Sort(symlinks)
		if !reflect.DeepEqual(symlinks, expectedSymlinks) {
			t.Error("symlinks do not match")
			t.Errorf("got  %+v", symlinks)
			t.Errorf("want %+v", expectedSymlinks)
		}
	})

	t.Run("does not restore backup and tablespaces when not calling --check", func(t *testing.T) {
		var called bool
		rsync.SetRsyncCommand(exectest.NewCommandWithVerifier(agent.Success, func(utility string, args ...string) {
			if !strings.HasSuffix(utility, "rsync") {
				t.Errorf("got %q want rsync", utility)
			}

			called = true
		}))
		defer rsync.ResetRsyncCommand()

		upgrade.SetPgUpgradeCommand(exectest.NewCommand(agent.Success))
		defer upgrade.ResetPgUpgradeCommand()

		opts := []*idl.PgOptions{{
			Role:          greenplum.PrimaryRole,
			Action:        idl.PgOptions_check,
			TargetVersion: "6.0.0",
		}}

		_, err := server.UpgradePrimaries(context.Background(), &idl.UpgradePrimariesRequest{Opts: opts})
		if err != nil {
			t.Fatalf("unexpected error %+v", err)
		}

		if called {
			t.Error("expected rsync to not be called")
		}
	})

	t.Run("errors when restoring the backup fails", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(agent.FailedRsync))
		defer rsync.ResetRsyncCommand()

		upgrade.SetPgUpgradeCommand(exectest.NewCommand(agent.Success))
		defer upgrade.ResetPgUpgradeCommand()

		opts := []*idl.PgOptions{
			{
				Role:          greenplum.PrimaryRole,
				Action:        idl.PgOptions_upgrade,
				TargetVersion: "6.0.0",
				NewDBID:       "1",
				NewDataDir:    "/new/data/dir",
				Tablespaces: map[int32]*idl.TablespaceInfo{
					1663: {Name: "tblspc1", Location: "/tmp/primary1/1663", UserDefined: true},
					1664: {Name: "tblspc2", Location: "/tmp/primary1/1664", UserDefined: true},
				},
			},
		}

		_, err := server.UpgradePrimaries(context.Background(), &idl.UpgradePrimariesRequest{Opts: opts})
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("got error %T, want type %T", err, exitErr)
		}

		if exitErr.ExitCode() != 2 {
			t.Errorf("got exit code %d, want 2", exitErr.ExitCode())
		}
	})

	t.Run("errors when restoring tablespaces fails", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(agent.Success))
		defer rsync.ResetRsyncCommand()

		upgrade.SetPgUpgradeCommand(exectest.NewCommand(agent.Success))
		defer upgrade.ResetPgUpgradeCommand()

		opts := []*idl.PgOptions{
			{
				Role:          greenplum.PrimaryRole,
				Action:        idl.PgOptions_upgrade,
				TargetVersion: "6.0.0",
				OldDBID:       "1",
				NewDataDir:    "/new/data/dir",
				Tablespaces: map[int32]*idl.TablespaceInfo{
					1663: {Name: "tblspc1", Location: "/tmp/primary1/1663", UserDefined: true},
					1664: {Name: "tblspc2", Location: "/tmp/primary1/1664", UserDefined: true},
				},
			},
		}

		_, err := server.UpgradePrimaries(context.Background(), &idl.UpgradePrimariesRequest{Opts: opts})
		var linkErr *os.LinkError
		if !errors.As(err, &linkErr) {
			t.Errorf("got error %T, want type %T", err, linkErr)
		}
	})

	t.Run("errors when pg_upgrade fails", func(t *testing.T) {
		utils.System.Hostname = func() (string, error) {
			return "sdw1", nil
		}

		rsync.SetRsyncCommand(exectest.NewCommand(agent.Success))
		defer rsync.ResetRsyncCommand()

		upgrade.SetPgUpgradeCommand(exectest.NewCommand(agent.FailedMain))
		defer upgrade.ResetPgUpgradeCommand()

		opts := []*idl.PgOptions{
			{Role: greenplum.PrimaryRole, Action: idl.PgOptions_upgrade, TargetVersion: "6.0.0", ContentID: 1, OldDBID: "1"},
			{Role: greenplum.PrimaryRole, Action: idl.PgOptions_upgrade, TargetVersion: "6.0.0", ContentID: 2, OldDBID: "2"},
		}

		_, err := server.UpgradePrimaries(context.Background(), &idl.UpgradePrimariesRequest{Opts: opts})
		var errs errorlist.Errors
		if !xerrors.As(err, &errs) {
			t.Errorf("error %T does not contain type %T", err, errs)
		}

		if len(errs) != len(opts) {
			t.Fatalf("got error count %d, want %d", len(errs), len(opts))
		}

		sort.Sort(errs)
		for i, err := range errs {
			expected := fmt.Sprintf("upgrade primary on host sdw1 with content %d: exit status 1", i+1)
			if err.Error() != expected {
				t.Errorf("got %q want %q", err.Error(), expected)
			}
		}
	})
}

func TestRestoreTablespaces(t *testing.T) {
	testhelper.SetupTestLogger()

	t.Run("restores user defined tablespaces", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(agent.Success))
		defer rsync.ResetRsyncCommand()

		utils.System.Symlink = func(oldname, newname string) error {
			return nil
		}
		defer utils.ResetSystemFunctions()

		tablespaces := map[int32]*idl.TablespaceInfo{
			1663: {Name: "tblspc1", Location: "/tmp/primary1/1663", UserDefined: true},
			1664: {Name: "tblspc2", Location: "/tmp/primary1/1664", UserDefined: true},
		}

		err := agent.RestoreTablespaces(tablespaces, "2", "/new/data/dir")
		if err != nil {
			t.Fatalf("unexpected error %+v", err)
		}
	})

	t.Run("skips restoring default tablespaces and only restores user defined tablespaces", func(t *testing.T) {
		expected := "/tmp/primary1/1664/2"

		rsync.SetRsyncCommand(exectest.NewCommandWithVerifier(agent.Success, func(utility string, args ...string) {
			if !strings.HasSuffix(utility, "rsync") {
				t.Errorf("got %q want rsync", utility)
			}

			if args[3] != expected {
				t.Errorf("got %q want %q", args[3], expected)
			}
		}))
		defer rsync.ResetRsyncCommand()

		utils.System.Lstat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}
		defer utils.ResetSystemFunctions()

		utils.System.Remove = func(name string) error {
			return nil
		}
		defer utils.ResetSystemFunctions()

		utils.System.Symlink = func(sourceDir, symLinkName string) error {
			return nil
		}
		defer utils.ResetSystemFunctions()

		tablespaces := map[int32]*idl.TablespaceInfo{
			1663: {Name: "tblspc1", Location: "/tmp/primary1/1663", UserDefined: false},
			1664: {Name: "tblspc2", Location: "/tmp/primary1/1664", UserDefined: true},
		}

		err := agent.RestoreTablespaces(tablespaces, "2", "/new/data/dir")
		if err != nil {
			t.Fatalf("unexpected error %+v", err)
		}
	})

	t.Run("errors when parse dbID fails", func(t *testing.T) {
		err := agent.RestoreTablespaces(nil, "", "")
		var expected *strconv.NumError
		if !errors.As(err, &expected) {
			t.Errorf("got error type %T want %T", err, expected)
		}
	})

	t.Run("errors when rsync fails", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(agent.FailedRsync))
		defer rsync.ResetRsyncCommand()

		tablespaces := map[int32]*idl.TablespaceInfo{
			1664: {Name: "tblspc2", Location: "/tmp/primary1/1664", UserDefined: true},
		}

		err := agent.RestoreTablespaces(tablespaces, "2", "/new/data/dir")
		var expected rsync.RsyncError
		if !errors.As(err, &expected) {
			t.Errorf("got error type %T want %T", err, expected)
		}
	})

	t.Run("errors when recreating the symlink fails to read the link", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(agent.Success))
		defer rsync.ResetRsyncCommand()

		expected := errors.New("oops")
		utils.System.Lstat = func(name string) (os.FileInfo, error) {
			return nil, expected
		}
		defer utils.ResetSystemFunctions()

		tablespaces := map[int32]*idl.TablespaceInfo{
			1664: {Name: "tblspc2", Location: "/tmp/primary1/1664", UserDefined: true},
		}

		err := agent.RestoreTablespaces(tablespaces, "2", "/new/data/dir")
		if !errors.Is(err, expected) {
			t.Errorf("got %#v want %#v", err, expected.Error())
		}
	})

	t.Run("errors when recreating the symlink fails to remove the link", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(agent.Success))
		defer rsync.ResetRsyncCommand()

		utils.System.Lstat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}
		defer utils.ResetSystemFunctions()

		expected := errors.New("oops")
		utils.System.Remove = func(name string) error {
			return expected
		}
		defer utils.ResetSystemFunctions()

		tablespaces := map[int32]*idl.TablespaceInfo{
			1664: {Name: "tblspc2", Location: "/tmp/primary1/1664", UserDefined: true},
		}

		err := agent.RestoreTablespaces(tablespaces, "2", "/new/data/dir")
		if !errors.Is(err, expected) {
			t.Errorf("got %#v want %#v", err, expected)
		}
	})

	t.Run("errors when recreating the symlink fails to create the link", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(agent.Success))
		defer rsync.ResetRsyncCommand()

		utils.System.Lstat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}
		defer utils.ResetSystemFunctions()

		utils.System.Remove = func(name string) error {
			return nil
		}
		defer utils.ResetSystemFunctions()

		expected := errors.New("oops")
		utils.System.Symlink = func(sourceDir, symLinkName string) error {
			return expected
		}
		defer utils.ResetSystemFunctions()

		tablespaces := map[int32]*idl.TablespaceInfo{
			1664: {Name: "tblspc2", Location: "/tmp/primary1/1664", UserDefined: true},
		}

		err := agent.RestoreTablespaces(tablespaces, "2", "/new/data/dir")
		if !errors.Is(err, expected) {
			t.Errorf("got %#v want %#v", err, expected)
		}
	})
}

type Symlink struct {
	Oldname string
	Newname string
}

type Symlinks []Symlink

func (s Symlinks) Len() int {
	return len(s)
}

func (s Symlinks) Less(i, j int) bool {
	return s[i].Oldname < s[j].Oldname && s[i].Newname < s[j].Newname
}

func (s Symlinks) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
