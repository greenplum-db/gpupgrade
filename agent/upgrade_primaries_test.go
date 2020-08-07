// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"errors"
	"io/ioutil"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

func ResetCommands() {
	agent.SetExecCommand(nil)
	rsync.SetRsyncCommand(nil)
}

func TestUpgradePrimary(t *testing.T) {
	testhelper.SetupTestLogger()

	// Disable exec.Command. This way, if a test forgets to mock it out, we
	// crash the test instead of executing code on a dev system.
	agent.SetExecCommand(nil)

	// We need a real temporary directory to change to. Replace MkdirAll() so
	// that we can make sure the directory is the correct one.
	tempDir, err := ioutil.TempDir("", "gpupgrade")
	if err != nil {
		t.Fatalf("creating temporary directory: %+v", err)
	}
	defer os.RemoveAll(tempDir)

	utils.System.MkdirAll = func(path string, perms os.FileMode) error {
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

	pairs := []*idl.DataDirPair{
		{
			SourceDataDir: "/data/old",
			TargetDataDir: "/data/new",
			SourcePort:    15432,
			TargetPort:    15433,
			Content:       1,
			DBID:          2,
		},
		{
			SourceDataDir: "/other/data/old",
			TargetDataDir: "/other/data/new",
			SourcePort:    99999,
			TargetPort:    88888,
			Content:       7,
			DBID:          6,
		},
	}

	// NOTE: we could choose to duplicate the upgrade.Run unit tests for all of
	// this, but we choose to instead rely on end-to-end tests for most of this
	// functionality, and test only a few integration paths here.

	t.Run("when pg_upgrade --check fails it returns an error", func(t *testing.T) {
		agent.SetExecCommand(exectest.NewCommand(agent.FailedMain))
		rsync.SetRsyncCommand(exectest.NewCommand(agent.Success))

		defer ResetCommands()

		request := &idl.UpgradePrimariesRequest{
			SourceBinDir: "/old/bin",
			TargetBinDir: "/new/bin",
			DataDirPairs: pairs,
			CheckOnly:    true,
			UseLinkMode:  false,
		}
		err := agent.UpgradePrimaries(tempDir, request)
		if err == nil {
			t.Fatal("UpgradeSegments() returned no error")
		}

		// XXX it'd be nice if we didn't couple against a hardcoded string here,
		// but it's difficult to unwrap multierror with the new xerrors
		// interface.
		if !strings.Contains(err.Error(), "check primary on host") ||
			!strings.Contains(err.Error(), "with content 1") {
			t.Errorf("error %q did not contain expected contents 'check primary on host' and 'content 1'",
				err.Error())
		}
	})

	t.Run("when pg_upgrade with no check fails it returns an error", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(agent.Success))
		agent.SetExecCommand(exectest.NewCommand(agent.FailedMain))
		defer ResetCommands()

		request := &idl.UpgradePrimariesRequest{
			SourceBinDir: "/old/bin",
			TargetBinDir: "/new/bin",
			DataDirPairs: pairs,
			CheckOnly:    false,
			UseLinkMode:  false}
		err := agent.UpgradePrimaries(tempDir, request)
		if err == nil {
			t.Fatal("UpgradeSegments() returned no error")
		}

		// XXX it'd be nice if we didn't couple against a hardcoded string here,
		// but it's difficult to unwrap multierror with the new xerrors
		// interface.
		if !strings.Contains(err.Error(), "upgrade primary on host") ||
			!strings.Contains(err.Error(), "with content 1") {
			t.Errorf("error %q did not contain expected contents 'upgrade primary on host' and 'content 1'",
				err.Error())
		}
	})

	t.Run("it does not perform a copy of the master backup directory when using check mode", func(t *testing.T) {
		agent.SetExecCommand(exectest.NewCommand(agent.Success))

		defer ResetCommands()

		request := buildRequest(pairs)
		request.CheckOnly = true

		rsync.SetRsyncCommand(
			exectest.NewCommandWithVerifier(agent.Success, func(commandName string, _ ...string) {
				if commandName == "rsync" {
					t.Error("unexpected rsync call")
				}
			}))

		_ = agent.UpgradePrimaries(tempDir, request)
	})

	t.Run("it returns errors in parallel if the copy step fails", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(agent.FailedRsync))
		agent.SetExecCommand(exectest.NewCommand(agent.Success))

		request := buildRequest(pairs)
		err = agent.UpgradePrimaries(tempDir, request)

		// We expect each part of the request to return its own ExitError,
		// containing the expected message from FailedRsync.
		var multiErr *multierror.Error
		if !errors.As(err, &multiErr) {
			t.Fatalf("got error %#v, want type %T", err, multiErr)
		}

		if len(multiErr.Errors) != len(pairs) {
			t.Errorf("received %d errors, want %d", len(multiErr.Errors), len(pairs))
		}

		for _, err := range multiErr.Errors {
			if !strings.Contains(string(err.Error()), "rsync failed cause I said so") {
				t.Errorf("wanted error message 'rsync failed cause I said so' from rsync, got %q",
					string(err.Error()))
			}
		}
	})

	t.Run("it grabs a copy of the master backup directory before running upgrade", func(t *testing.T) {
		defer ResetCommands()

		var targetDataDirs []string
		var targetDataDirsUsed []string

		for _, pair := range pairs {
			targetDataDirs = append(targetDataDirs, pair.TargetDataDir)
		}

		targetDataDirsUsedChannel := make(chan string, len(targetDataDirs))

		agent.SetExecCommand(exectest.NewCommand(agent.Success))
		rsync.SetRsyncCommand(exectest.NewCommandWithVerifier(agent.Success, func(utility string, arguments ...string) {
			call := rsyncCall(utility, arguments)

			if call.sourceDir != "/some/master/backup/dir/" {
				t.Errorf("rsync source directory was %v, want %v",
					call.sourceDir,
					"/some/master/backup/dir")
			}

			for _, dir := range targetDataDirs {
				if call.targetDir == dir {
					targetDataDirsUsedChannel <- dir
				}
			}

			expectedExclusions := []string{
				"--exclude", "internal.auto.conf",
				"--exclude", "postgresql.conf",
				"--exclude", "pg_hba.conf",
				"--exclude", "postmaster.opts",
				"--exclude", "gp_dbid",
				"--exclude", "gpssh.conf",
				"--exclude", "gpperfmon",
			}

			if !reflect.DeepEqual(call.exclusions, expectedExclusions) {
				t.Errorf("got %q exclusions in rsync, want %q",
					call.exclusions,
					expectedExclusions)
			}
		}))

		request := buildRequest(pairs)
		request.MasterBackupDir = "/some/master/backup/dir"

		err := agent.UpgradePrimaries(tempDir, request)
		if err != nil {
			t.Error(err)
		}

		close(targetDataDirsUsedChannel)

		//
		// Collect data dirs that were used to restore
		// a backup of the master data directory
		//
		// note: we're using channels because the rsync is happening
		// on a goroutine, so order cannot be guaranteed
		//
		for dir := range targetDataDirsUsedChannel {
			targetDataDirsUsed = append(targetDataDirsUsed, dir)
		}

		sort.Strings(targetDataDirs)
		sort.Strings(targetDataDirsUsed)
		if !reflect.DeepEqual(targetDataDirsUsed, targetDataDirs) {
			t.Errorf("all target data directories (%q) should have been upgraded, only %q were",
				targetDataDirs,
				targetDataDirsUsed)
		}
	})
}

type rsyncRequest struct {
	commandName string
	sourceDir   string
	targetDir   string
	archiveFlag string
	deleteFlag  string
	exclusions  []string
}

func rsyncCall(utility string, arguments []string) rsyncRequest {
	r := rsyncRequest{}
	r.commandName = utility

	if len(arguments) > 0 {
		r.archiveFlag = arguments[0]
	}

	if len(arguments) > 1 {
		r.deleteFlag = arguments[1]
	}

	if len(arguments) > 2 {
		r.sourceDir = arguments[2]
	}

	if len(arguments) > 3 {
		r.targetDir = arguments[3]
	}

	if len(arguments) > 4 {
		r.exclusions = arguments[4:]
	}

	return r
}

func buildRequest(pairs []*idl.DataDirPair) *idl.UpgradePrimariesRequest {
	return &idl.UpgradePrimariesRequest{
		SourceBinDir:    "/old/bin",
		TargetBinDir:    "/new/bin",
		DataDirPairs:    pairs,
		CheckOnly:       false,
		UseLinkMode:     false,
		MasterBackupDir: "/some/master/backup/dir",
	}
}
