// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"context"
	"errors"
	"os/exec"
	"os/user"
	"path/filepath"
	"testing"
	"time"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestArchiveLogDirectories(t *testing.T) {
	testlog.SetupTestLogger()
	agentServer := agent.New()

	t.Run("archives log directory on segment hosts", func(t *testing.T) {
		homeDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, homeDir)

		utils.System.Current = func() (*user.User, error) {
			return &user.User{HomeDir: homeDir}, nil
		}

		logDir := filepath.Join(homeDir, "gpAdminLogs", "gpupgrade")
		testutils.MustCreateDir(t, logDir)
		defer testutils.MustRemoveAll(t, logDir)

		var upgradeID string
		logArchiveDir := hub.GetLogArchiveDir(logDir, upgradeID, time.Now())
		defer testutils.MustRemoveAll(t, logArchiveDir)

		_, err := agentServer.ArchiveLogDirectory(context.Background(), &idl.ArchiveLogDirectoryRequest{LogArchiveDir: logArchiveDir})
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}

		testutils.PathMustNotExist(t, logDir)
		testutils.PathMustExist(t, logArchiveDir)
	})

	t.Run("errors when failing to archive log directory on segment host", func(t *testing.T) {
		logArchiveDir := "" // use an empty target directory string to force an error
		_, err := agentServer.ArchiveLogDirectory(context.Background(), &idl.ArchiveLogDirectoryRequest{LogArchiveDir: logArchiveDir})
		if err == nil {
			t.Errorf("expected error")
		}

		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Errorf("got %T want %T", err, exitError)
		}
	})
}
