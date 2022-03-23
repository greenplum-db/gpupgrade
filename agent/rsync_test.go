// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

func TestRsync(t *testing.T) {
	testlog.SetupLogger()
	server := agent.NewServer(agent.Config{})

	source := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, source)

	destination := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, destination)

	for _, file := range upgrade.PostgresFiles {
		path := filepath.Join(source, file)
		testutils.MustWriteToFile(t, path, "")

		path = filepath.Join(destination, file)
		testutils.MustWriteToFile(t, path, "")
	}

	t.Run("successfully rsyncs data directories", func(t *testing.T) {
		var options = []string{"--archive", "--compress", "--stats"}
		var excludes = []string{"pg_hba.conf", "postmaster.opts"}

		defer rsync.SetRsyncCommand(exec.Command)
		rsync.SetRsyncCommand(exectest.NewCommandWithVerifier(agent.Success, func(utility string, args ...string) {
			if !strings.HasSuffix(utility, "rsync") {
				t.Errorf("got %q want rsync", utility)
			}

			options := args[:3]
			if !reflect.DeepEqual(options, hub.Options) {
				t.Errorf("got options %q want %q", options, hub.Options)
			}

			src := args[3]
			expected := source + string(os.PathSeparator)
			if src != expected {
				t.Errorf("got source %q want %q", src, expected)
			}

			dst := args[4]
			expected = "sdw1:" + destination
			if dst != expected {
				t.Errorf("got destination %q want %q", dst, expected)
			}

			exclusions := strings.Join(args[6:], " ")
			expected = strings.Join(excludes, " --exclude ")
			if !reflect.DeepEqual(exclusions, expected) {
				t.Errorf("got exclusions %q want %q", exclusions, expected)
			}
		}))

		request := &idl.RsyncRequest{
			Options: []*idl.RsyncRequest_RsyncOptions{{
				Sources:         []string{source + string(os.PathSeparator)},
				DestinationHost: "sdw1",
				Destination:     destination,
				Options:         options,
				ExcludedFiles:   excludes,
			}},
		}

		_, err := server.RsyncDataDirectories(context.Background(), request)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("errors when source data directory is empty", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(agent.Success))
		defer rsync.ResetRsyncCommand()

		source := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, source)

		request := &idl.RsyncRequest{Options: []*idl.RsyncRequest_RsyncOptions{
			{Sources: []string{source}, Destination: destination},
		}}

		_, err := server.RsyncDataDirectories(context.Background(), request)
		if err == nil {
			t.Errorf("expected an error")
		}

		var invalid *upgrade.InvalidDataDirectoryError
		if !errors.As(invalid, &err) {
			t.Errorf("got type %T want %T", err, invalid)
		}
	})

	t.Run("errors when source data directory is invalid", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(agent.Success))
		defer rsync.ResetRsyncCommand()

		dir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir)

		// create invalid directory that omits postgres verification files
		testutils.MustWriteToFile(t, filepath.Join(dir, "foo.txt"), "")
		err := os.Mkdir(filepath.Join(dir, "bar"), 0700)
		if err != nil {
			t.Fatalf("creating bar directory: %v", err)
		}

		request := &idl.RsyncRequest{Options: []*idl.RsyncRequest_RsyncOptions{
			{Sources: []string{dir}, Destination: destination},
		}}

		_, err = server.RsyncDataDirectories(context.Background(), request)
		if err == nil {
			t.Errorf("expected an error")
		}

		var invalid *upgrade.InvalidDataDirectoryError
		if !errors.As(invalid, &err) {
			t.Errorf("got type %T want %T", err, invalid)
		}
	})

	t.Run("errors when multiple rsync calls fail", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(agent.FailedRsync))
		defer rsync.ResetRsyncCommand()

		request := &idl.RsyncRequest{Options: []*idl.RsyncRequest_RsyncOptions{
			{Sources: []string{source}, Destination: destination},
			{Sources: []string{source}, Destination: destination},
		}}

		_, err := server.RsyncDataDirectories(context.Background(), request)
		if err == nil {
			t.Error("expected error, returned nil")
		}

		var errs errorlist.Errors
		if !errors.As(err, &errs) {
			t.Fatalf("got error %#v, want type %T", err, errs)
		}

		if len(errs) != 2 {
			t.Errorf("got %d errors want 2", len(errs))
		}

		for _, err := range errs {
			var rsyncError rsync.RsyncError
			if !errors.As(err, &rsyncError) {
				t.Errorf("got type %T want %T", err, rsyncError)
			}
		}
	})
}

func TestRsyncTablespaceDirectories(t *testing.T) {
	testlog.SetupLogger()
	server := agent.NewServer(agent.Config{})

	sourceTsLocationDir := "/filespace/demoDataDir0/16386"
	utils.System.DirFS = func(dir string) fs.FS {
		return fstest.MapFS{
			filepath.Join("12094", upgrade.PGVersion): {},
			filepath.Join("12094", "16384"):           {},
		}
	}
	defer func() {
		utils.System.DirFS = os.DirFS
	}()

	destination := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, destination)

	t.Run("successfully rsyncs tablespace directories", func(t *testing.T) {
		var options = []string{"--archive", "--compress", "--stats"}
		var excludes = []string{"pg_hba.conf", "postmaster.opts"}

		defer rsync.SetRsyncCommand(exec.Command)
		rsync.SetRsyncCommand(exectest.NewCommandWithVerifier(agent.Success, func(utility string, args ...string) {
			if !strings.HasSuffix(utility, "rsync") {
				t.Errorf("got %q want rsync", utility)
			}

			options := args[:3]
			if !reflect.DeepEqual(options, hub.Options) {
				t.Errorf("got options %q want %q", options, hub.Options)
			}

			src := args[3]
			expected := sourceTsLocationDir + string(os.PathSeparator)
			if src != expected {
				t.Errorf("got source %q want %q", src, expected)
			}

			dst := args[4]
			expected = "sdw1:" + destination
			if dst != expected {
				t.Errorf("got destination %q want %q", dst, expected)
			}

			exclusions := strings.Join(args[6:], " ")
			expected = strings.Join(excludes, " --exclude ")
			if !reflect.DeepEqual(exclusions, expected) {
				t.Errorf("got exclusions %q want %q", exclusions, expected)
			}
		}))

		request := &idl.RsyncRequest{
			Options: []*idl.RsyncRequest_RsyncOptions{{
				Sources:         []string{sourceTsLocationDir + string(os.PathSeparator)},
				DestinationHost: "sdw1",
				Destination:     destination,
				Options:         options,
				ExcludedFiles:   excludes,
			}},
		}

		_, err := server.RsyncTablespaceDirectories(context.Background(), request)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("errors when failing to verify tablespace directory", func(t *testing.T) {
		var rsyncCalled bool
		defer rsync.SetRsyncCommand(exec.Command)
		rsync.SetRsyncCommand(exectest.NewCommandWithVerifier(agent.Success, func(utility string, args ...string) {
			rsyncCalled = true
		}))

		invalidTablespaceDir := "/filespace/demoDataDir0/16386"
		utils.System.DirFS = func(dir string) fs.FS {
			return fstest.MapFS{
				// create an invalid tablespace directory by removing PG_VERSION
				filepath.Join("12094", "16384"): {},
			}
		}
		defer func() {
			utils.System.DirFS = os.DirFS
		}()

		request := &idl.RsyncRequest{Options: []*idl.RsyncRequest_RsyncOptions{
			{Sources: []string{invalidTablespaceDir}, Destination: destination},
		}}

		_, err := server.RsyncTablespaceDirectories(context.Background(), request)
		expected := fmt.Sprintf("invalid tablespace directory %q", filepath.Join(invalidTablespaceDir, "12094"))
		if err.Error() != expected {
			t.Errorf("got error %#v want %#v", err, expected)
		}

		if rsyncCalled {
			t.Errorf("expected rsync to not be called")
		}
	})
}
