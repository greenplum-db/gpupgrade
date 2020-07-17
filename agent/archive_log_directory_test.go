// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"context"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestArchiveLogDirectories(t *testing.T) {
	testhelper.SetupTestLogger()
	server := agent.NewServer(agent.Config{})

	t.Run("bubbles up errors", func(t *testing.T) {
		// empty target directory string to force an error
		newDir := ""
		_, err := server.ArchiveLogDirectory(context.Background(), &idl.ArchiveLogDirectoryRequest{NewDir: newDir})
		if err == nil {
			t.Errorf("expected error")
		}
		var exitError *exec.ExitError
		if !xerrors.As(err, &exitError) {
			t.Errorf("got %T, want %T", err, exitError)
		}
	})

	t.Run("archives log directories", func(t *testing.T) {
		homeDir := testutils.GetTempDir(t, "")

		mockUser := user.User{HomeDir: homeDir}
		utils.System.CurrentUser = func() (*user.User, error) {
			return &mockUser, nil
		}
		oldLogDir := filepath.Join(mockUser.HomeDir, "gpAdminLogs", "gpupgrade")
		err := utils.System.MkdirAll(oldLogDir, 0700)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}
		defer os.RemoveAll(homeDir)

		newLogDir := oldLogDir + "xxxxxx"
		_, err = server.ArchiveLogDirectory(context.Background(), &idl.ArchiveLogDirectoryRequest{NewDir: newLogDir})
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}
		defer os.RemoveAll(newLogDir)

		_, err = os.Stat(oldLogDir)
		if !os.IsNotExist(err) {
			t.Errorf("old log dir %q must be removed", oldLogDir)
		}

		_, err = os.Stat(newLogDir)
		if err != nil {
			t.Errorf("got %#v, want nil", err)
		}
	})
}
