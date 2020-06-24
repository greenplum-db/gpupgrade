// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

func TestRsync(t *testing.T) {
	testhelper.SetupTestLogger()
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

	t.Run("successfully rsyncs", func(t *testing.T) {
		var options = []string{"--archive", "--compress", "--stats"}
		var excludes = []string{"pg_hba.conf", "postmaster.opts"}

		defer rsync.SetRsyncCommand(exec.Command)
		rsync.SetRsyncCommand(exectest.NewCommandWithVerifier(agent.Success, func(utility string, args ...string) {
			if utility != "rsync" {
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
			Pairs: []*idl.RsyncPair{{
				Source:      source,
				RemoteHost:  "sdw1",
				Destination: destination,
			}},
			Options:  options,
			Excludes: excludes,
		}

		_, err := server.Rsync(context.Background(), request)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("errors when source data directory is invalid", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(agent.Success))
		defer rsync.ResetRsyncCommand()

		// create invalid directory that omits postgres files
		source := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, source)

		request := &idl.RsyncRequest{Pairs: []*idl.RsyncPair{
			{Source: source, Destination: destination},
		}}

		_, err := server.Rsync(context.Background(), request)
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

		request := &idl.RsyncRequest{Pairs: []*idl.RsyncPair{
			{Source: source, Destination: destination},
			{Source: source, Destination: destination},
		}}

		_, err := server.Rsync(context.Background(), request)
		if err == nil {
			t.Error("expected error, returned nil")
		}

		var mErr *multierror.Error
		if !errors.As(err, &mErr) {
			t.Errorf("got type %T want %T", err, mErr)
		}

		if mErr.Len() != 2 {
			t.Errorf("got %d errors want 2", mErr.Len())
		}

		for _, err := range mErr.Errors {
			var rsyncError rsync.RsyncError
			if !errors.As(err, &rsyncError) {
				t.Errorf("got type %T want %T", err, rsyncError)
			}
		}
	})
}
