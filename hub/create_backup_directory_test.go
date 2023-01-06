// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestCreateBackupDirectory(t *testing.T) {
	testlog.SetupTestLogger()

	t.Run("creates backup directory if it does not exist", func(t *testing.T) {
		parentBackupDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, parentBackupDir)

		backupDir := filepath.Join(parentBackupDir, ".gpupgrade")
		testutils.PathMustNotExist(t, backupDir)

		err := hub.CreateBackupDirectory(backupDir)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		testutils.PathMustExist(t, backupDir)
	})

	t.Run("does not create backup directory if it already exists", func(t *testing.T) {
		backupDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, backupDir)

		testutils.PathMustExist(t, backupDir)

		err := hub.CreateBackupDirectory(backupDir)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		testutils.PathMustExist(t, backupDir)
	})

	t.Run("errors when creating backup directory fails", func(t *testing.T) {
		backupDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, backupDir)

		expected := os.ErrPermission
		utils.System.MkdirAll = func(name string, perm os.FileMode) error {
			return expected
		}
		defer utils.ResetSystemFunctions()

		err := hub.CreateBackupDirectory(backupDir)
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("got error %#v want %#v", err, os.ErrPermission)
		}

		testutils.PathMustExist(t, backupDir)
	})
}
