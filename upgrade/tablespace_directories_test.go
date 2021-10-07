//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package upgrade_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

// The default tablespace permissions with execute set to allow access to children
// directories and files.
const userRWX = 0700

func TestDeleteNewTablespaceDirectories(t *testing.T) {
	testlog.SetupLogger()
	utils.System.Hostname = func() (s string, err error) {
		return "", nil
	}
	defer func() {
		utils.System.Hostname = os.Hostname
	}()

	t.Run("deletes parent dbID directory when it's empty", func(t *testing.T) {
		tablespaceDir, dbIDDir, tsLocation := testutils.MustMakeTablespaceDir(t, 0)
		defer testutils.MustRemoveAll(t, tsLocation)

		err := upgrade.DeleteTablespaceDirectories(step.DevNullStream, []string{tablespaceDir})
		if err != nil {
			t.Errorf("DeleteTablespaceDirectories returned error %+v", err)
		}

		testutils.PathMustNotExist(t, tablespaceDir)
		testutils.PathMustNotExist(t, dbIDDir)
	})

	t.Run("rerun of DeleteTablespaceDirectories after previous successful execution succeeds", func(t *testing.T) {
		tablespaceDir, dbIdDir, tsLocation := testutils.MustMakeTablespaceDir(t, 0)
		defer testutils.MustRemoveAll(t, tsLocation)

		err := upgrade.DeleteTablespaceDirectories(step.DevNullStream, []string{tablespaceDir})
		if err != nil {
			t.Errorf("DeleteTablespaceDirectories returned error %+v", err)
		}

		err = upgrade.DeleteTablespaceDirectories(step.DevNullStream, []string{tablespaceDir})
		if err != nil {
			t.Errorf("rerun of DeleteTablespaceDirectories returned error %+v", err)
		}

		testutils.PathMustNotExist(t, tablespaceDir)
		testutils.PathMustNotExist(t, dbIdDir)
	})

	t.Run("does not delete parent dbID directory when it's not empty", func(t *testing.T) {
		tablespaceDir, dbIDDir, tsLocation := testutils.MustMakeTablespaceDir(t, 0)
		defer testutils.MustRemoveAll(t, tsLocation)

		relfileNode := filepath.Join(dbIDDir, "16389")
		testutils.MustWriteToFile(t, relfileNode, "")

		err := upgrade.DeleteTablespaceDirectories(step.DevNullStream, []string{tablespaceDir})
		if err != nil {
			t.Errorf("DeleteTablespaceDirectories returned error %+v", err)
		}

		testutils.PathMustNotExist(t, tablespaceDir)
		testutils.PathMustExist(t, dbIDDir)
	})

	t.Run("deletes multiple tablespace directories including their parent dbID directory when empty", func(t *testing.T) {
		type TablespaceDirs struct {
			tablespaceDir string
			dbIDDir       string
		}
		var dirs []TablespaceDirs
		var tsDirs []string

		tablespaceOids := []int{16386, 16387, 16388}
		for _, oid := range tablespaceOids {
			tablespaceDir, dbIdDir, tsLocation := testutils.MustMakeTablespaceDir(t, oid)
			defer testutils.MustRemoveAll(t, tsLocation)

			dirs = append(dirs, TablespaceDirs{tablespaceDir, dbIdDir})
			tsDirs = append(tsDirs, tablespaceDir)
		}

		err := upgrade.DeleteTablespaceDirectories(step.DevNullStream, tsDirs)
		if err != nil {
			t.Errorf("DeleteTablespaceDirectories returned error %+v", err)
		}

		for _, dir := range dirs {
			testutils.PathMustNotExist(t, dir.tablespaceDir)
			testutils.PathMustNotExist(t, dir.dbIDDir)
		}
	})

	t.Run("errors when tablespace directory is invalid", func(t *testing.T) {
		tablespaceDir, _, tsLocation := testutils.MustMakeTablespaceDir(t, 0)
		defer testutils.MustRemoveAll(t, tsLocation)

		invalidDbIDDir := testutils.GetTempDir(t, "invalidDbIDDir")
		defer testutils.MustRemoveAll(t, invalidDbIDDir)

		invalidTablspaceDir := filepath.Join(invalidDbIDDir, "invalidTablspaceDir")
		testutils.MustCreateDir(t, invalidTablspaceDir)

		dirs := []string{tablespaceDir, invalidTablspaceDir}
		err := upgrade.DeleteTablespaceDirectories(step.DevNullStream, dirs)
		expected := fmt.Sprintf("Invalid tablespace directory. Expected %q to start with 'GPDB_'.", invalidTablspaceDir)
		if err.Error() != expected {
			t.Errorf("got error %#v\n want %#v", err, expected)
		}

		for _, dir := range dirs {
			dbIdDir := filepath.Dir(filepath.Clean(dir))
			testutils.PathMustExist(t, dir)
			testutils.PathMustExist(t, dbIdDir)
		}
	})

	t.Run("errors when tablespace directory can't be deleted", func(t *testing.T) {
		tablespaceDir, dbIDDir, tsLocation := testutils.MustMakeTablespaceDir(t, 0)
		defer func() {
			err := os.Chmod(dbIDDir, userRWX)
			if err != nil {
				t.Fatalf("making parent dbId directory writeable: %v", err)
			}
			testutils.MustRemoveAll(t, tsLocation)
		}()

		// Set parent dbID directory to read only so its children cannot be
		// removed.
		err := os.Chmod(dbIDDir, 0500)
		if err != nil {
			t.Fatalf("making parent dbID directory read only: %v", err)
		}

		err = upgrade.DeleteTablespaceDirectories(step.DevNullStream, []string{tablespaceDir})

		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("got error %#v want %#v", err, os.ErrPermission)
		}

		testutils.PathMustExist(t, tablespaceDir)
		testutils.PathMustExist(t, dbIDDir)
	})

	t.Run("errors when failing to read parent dbID directory", func(t *testing.T) {
		tablespaceDir, dbIDDir, tsLocation := testutils.MustMakeTablespaceDir(t, 0)
		defer func() {
			err := os.Chmod(dbIDDir, userRWX)
			if err != nil {
				t.Fatalf("making parent dbID directory writeable: %v", err)
			}
			testutils.MustRemoveAll(t, tsLocation)
		}()

		// Set parent dbid directory to write and execute to allow its children
		// to be removed, but does not allow its contents to be read.
		err := os.Chmod(dbIDDir, 0300)
		if err != nil {
			t.Fatalf("making parent directory read only: %v", err)
		}

		err = upgrade.DeleteTablespaceDirectories(step.DevNullStream, []string{tablespaceDir})
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("got error %#v want %#v", err, os.ErrPermission)
		}

		testutils.PathMustExist(t, tablespaceDir)
		testutils.PathMustExist(t, dbIDDir)
	})

	t.Run("errors when failing to remove parent dbID directory", func(t *testing.T) {
		tablespaceDir, dbIDDir, tsLocation := testutils.MustMakeTablespaceDir(t, 0)
		defer func() {
			err := os.Chmod(tsLocation, userRWX)
			if err != nil {
				t.Fatalf("making tablespace location writeable: %v", err)
			}
			testutils.MustRemoveAll(t, tsLocation)
		}()

		// Set tablespace location to read and execute to allow its children
		// to be removed.
		err := os.Chmod(tsLocation, 0500)
		if err != nil {
			t.Fatalf("making tablespace location directory read only: %v", err)
		}

		err = upgrade.DeleteTablespaceDirectories(step.DevNullStream, []string{tablespaceDir})
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("got error %#v want %#v", err, os.ErrPermission)
		}

		testutils.PathMustNotExist(t, tablespaceDir)
		testutils.PathMustExist(t, dbIDDir)
	})

	t.Run("rerun finishes successfully", func(t *testing.T) {
		type TablespaceDirs struct {
			tablespaceDir string
			dbIdDir       string
		}
		var dirs []TablespaceDirs
		var tsDirs []string

		tablespaceOids := []int{16386, 16387, 16388}
		for _, oid := range tablespaceOids {
			tablespaceDir, dbIdDir, tsLocation := testutils.MustMakeTablespaceDir(t, oid)
			defer testutils.MustRemoveAll(t, tsLocation)

			dirs = append(dirs, TablespaceDirs{tablespaceDir, dbIdDir})
			tsDirs = append(tsDirs, tablespaceDir)
		}

		err := upgrade.DeleteTablespaceDirectories(step.DevNullStream, tsDirs)
		if err != nil {
			t.Errorf("DeleteTablespaceDirectories returned error %+v", err)
		}

		for _, dir := range dirs {
			testutils.PathMustNotExist(t, dir.tablespaceDir)
			testutils.PathMustNotExist(t, dir.dbIdDir)
		}

		err = upgrade.DeleteTablespaceDirectories(step.DevNullStream, tsDirs)
		if err != nil {
			t.Errorf("unexpected error %+v", err)
		}
	})
}

func TestVerifyTablespaceDirectories(t *testing.T) {
	t.Run("succeeds when given multiple legacy and non-legacy tablespace locations", func(t *testing.T) {
		var dirs []string

		// create legacy tablespace directories
		tablespaceOids := []int{16386, 16387, 16388}
		for _, oid := range tablespaceOids {
			_, tsLocationDir := testutils.MustMake5XTablespaceDir(t, oid)
			defer testutils.MustRemoveAll(t, tsLocationDir)

			dirs = append(dirs, tsLocationDir)
		}

		// create tablespace directories
		_, _, tsLocation1 := testutils.MustMakeTablespaceDir(t, 12812)
		defer testutils.MustRemoveAll(t, tsLocation1)
		dirs = append(dirs, tsLocation1)

		_, _, tsLocation2 := testutils.MustMakeTablespaceDir(t, 12094)
		defer testutils.MustRemoveAll(t, tsLocation2)
		dirs = append(dirs, tsLocation2)

		err := upgrade.VerifyTablespaceDirectories(dirs)
		if err != nil {
			t.Errorf("unexpected error %+v", err)
		}
	})

	t.Run("does not error when tablespace directory contains other files in additional to tablespace directory", func(t *testing.T) {
		_, _, tsLocation := testutils.MustMakeTablespaceDir(t, 12812)
		defer testutils.MustRemoveAll(t, tsLocation)

		testutils.MustWriteToFile(t, filepath.Join(tsLocation, "foo"), "")

		err := upgrade.VerifyTablespaceDirectories([]string{tsLocation})
		if err != nil {
			t.Errorf("unexpected error %+v", err)
		}
	})

	t.Run("errors when failing to read tablespace directory", func(t *testing.T) {
		tsLocationDir := testutils.GetTempDir(t, "")
		defer func() {
			err := os.Chmod(tsLocationDir, userRWX)
			if err != nil {
				t.Fatalf("making tablespace location directory writeable: %v", err)
			}
			testutils.MustRemoveAll(t, tsLocationDir)
		}()

		// Set tablespace directory to write and execute to prevent its contents
		// from being read.
		err := os.Chmod(tsLocationDir, 0300)
		if err != nil {
			t.Fatalf("making tablespace directory non-readable: %v", err)
		}

		err = upgrade.VerifyTablespaceDirectories([]string{tsLocationDir})
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("got error %#v want %#v", err, os.ErrPermission)
		}
	})

	t.Run("errors when there are no tablespace directories", func(t *testing.T) {
		tsLocation := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, tsLocation)

		err := upgrade.VerifyTablespaceDirectories([]string{tsLocation})
		expected := fmt.Sprintf("Invalid tablespace directory %q", tsLocation)
		if err.Error() != expected {
			t.Errorf("got error %#v want %#v", err, expected)
		}
	})

	t.Run("errors when failing to verify legacy tablespace directory", func(t *testing.T) {
		dbOidDir, tsLocation := testutils.MustMake5XTablespaceDir(t, 16386)
		defer testutils.MustRemoveAll(t, tsLocation)

		utils.System.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrPermission
		}
		defer func() {
			utils.System.Stat = os.Stat
		}()

		err := upgrade.VerifyTablespaceDirectories([]string{tsLocation})
		expected := xerrors.Errorf("checking legacy tablespace directory %q: %w", filepath.Join(dbOidDir, upgrade.PGVersion), os.ErrPermission)
		if err.Error() != expected.Error() {
			t.Errorf("got error %#v want %#v", err, expected)
		}
	})

	t.Run("errors when failing to verify non-legacy tablespace directory", func(t *testing.T) {
		tsLocation := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, tsLocation)

		someSubDir := filepath.Join(tsLocation, "1", "some_dub_dir")
		testutils.MustCreateDir(t, someSubDir)
		defer testutils.MustRemoveAll(t, someSubDir)

		err := upgrade.VerifyTablespaceDirectories([]string{tsLocation})
		expected := xerrors.Errorf("Invalid tablespace directory. Expected %q to start with 'GPDB_'.", someSubDir)
		if err.Error() != expected.Error() {
			t.Errorf("got error %#v want %#v", err, expected)
		}
	})

	t.Run("errors when directory is not a legacy and non-legacy tablespace directory", func(t *testing.T) {
		tablespaceDir, dbIdDir, tsLocation := testutils.MustMakeTablespaceDir(t, 16386)
		defer testutils.MustRemoveAll(t, tsLocation)

		testutils.MustRemoveAll(t, tablespaceDir)

		err := upgrade.VerifyTablespaceDirectories([]string{tsLocation})
		expected := xerrors.Errorf("Invalid tablespace directory %q", dbIdDir)
		if err.Error() != expected.Error() {
			t.Errorf("got error %#v want %#v", err, expected)
		}
	})
}

func TestVerifyLegacyTablespaceDirectory(t *testing.T) {
	t.Run("succeeds for legacy tablespace directories", func(t *testing.T) {
		_, tsLocation := testutils.MustMake5XTablespaceDir(t, 16386)
		defer testutils.MustRemoveAll(t, tsLocation)

		path := filepath.Join(tsLocation, "12094")
		exists, err := upgrade.VerifyLegacyTablespaceDirectory(path)
		if err != nil {
			t.Errorf("unexpected error %+v", err)
		}

		if !exists {
			t.Errorf("expected path %q to exist", path)
		}
	})

	t.Run("returns false for non-legacy tablespace directories", func(t *testing.T) {
		_, dbIdDir, tsLocation := testutils.MustMakeTablespaceDir(t, 16386)
		defer testutils.MustRemoveAll(t, tsLocation)

		exists, err := upgrade.VerifyLegacyTablespaceDirectory(dbIdDir)
		if err != nil {
			t.Errorf("unexpected error %+v", err)
		}

		if exists {
			t.Errorf("expected path %q to not exist", filepath.Join(dbIdDir, upgrade.PGVersion))
		}
	})

	t.Run("returns false when path does not exist", func(t *testing.T) {
		path := "/does/not/exist"
		exists, err := upgrade.VerifyLegacyTablespaceDirectory(path)
		if err != nil {
			t.Errorf("unexpected error %+v", err)
		}

		if exists {
			t.Errorf("expected path %q to not exist", filepath.Join(path, upgrade.PGVersion))
		}
	})

	t.Run("errors when failing to check if path exists", func(t *testing.T) {
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrPermission
		}
		defer func() {
			utils.System.Stat = os.Stat
		}()

		exists, err := upgrade.VerifyLegacyTablespaceDirectory("")
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("got error %#v want %#v", err, os.ErrPermission)
		}

		if exists {
			t.Errorf("expected path %q to not exist", "")
		}
	})
}

func TestVerifyTablespaceDirectory(t *testing.T) {
	t.Run("succeeds for tablespace directories", func(t *testing.T) {
		_, dbIdDir, tsLocation := testutils.MustMakeTablespaceDir(t, 16386)
		defer testutils.MustRemoveAll(t, tsLocation)

		exists, err := upgrade.VerifyTablespaceDirectory(dbIdDir)
		if err != nil {
			t.Errorf("unexpected error %+v", err)
		}

		if !exists {
			t.Errorf("expected path %q to exist", dbIdDir)
		}
	})

	t.Run("fails with legacy tablespace directories", func(t *testing.T) {
		_, tsLocation := testutils.MustMake5XTablespaceDir(t, 16386)
		defer testutils.MustRemoveAll(t, tsLocation)

		path := filepath.Join(tsLocation, "12094")
		exists, err := upgrade.VerifyTablespaceDirectory(path)
		if err != nil {
			t.Errorf("unexpected error %+v", err)
		}

		if exists {
			t.Errorf("expected path %q not exist", path)
		}
	})

	t.Run("fails if path does not look like tablespace directory", func(t *testing.T) {
		dbIDDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dbIDDir)

		subDir := filepath.Join(dbIDDir, "some_sub_dir")
		testutils.MustCreateDir(t, subDir)

		exists, err := upgrade.VerifyTablespaceDirectory(dbIDDir)
		expected := fmt.Sprintf("Invalid tablespace directory. Expected %q to start with 'GPDB_'.", subDir)
		if err.Error() != expected {
			t.Errorf("got error %#v want %#v", err, expected)
		}

		if exists {
			t.Errorf("expected path %q not exist", dbIDDir)
		}
	})

	t.Run("errors when failing to read directory", func(t *testing.T) {
		path := "/does/not/exist"
		exists, err := upgrade.VerifyTablespaceDirectory(path)
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("got error %#v want %#v", err, os.ErrNotExist)
		}

		if exists {
			t.Errorf("expected path %q to not exist", path)
		}
	})
}
