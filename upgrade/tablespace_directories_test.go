//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package upgrade_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

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
	t.Run("succeeds with multiple legacy tablespace locations", func(t *testing.T) {
		ts := fstest.MapFS{
			filepath.Join("12094", upgrade.PGVersion): {},
			filepath.Join("12094", "16384"):           {},
		}

		paths := []string{"/filespace/demoDataDir0/16386", "/filespace/demoDataDir0/16387", "/filespace/demoDataDir0/16388"}
		for _, path := range paths {
			err := upgrade.VerifyTablespaceLocation(ts, path)
			if err != nil {
				t.Errorf("unexpected error %+v", err)
			}
		}
	})

	t.Run("succeeds with multiple non-legacy tablespace locations", func(t *testing.T) {
		ts := fstest.MapFS{
			filepath.Join("1", "GPDB_6_301908232"): {Mode: os.ModeDir},
		}

		paths := []string{"/filespace/demoDataDir0/12812", "/filespace/demoDataDir0/12094"}
		for _, path := range paths {
			err := upgrade.VerifyTablespaceLocation(ts, path)
			if err != nil {
				t.Errorf("unexpected error %+v", err)
			}
		}
	})

	t.Run("does not error when tablespace directory contains additional files", func(t *testing.T) {
		ts := fstest.MapFS{
			"foo":                                  {},
			filepath.Join("1", "bar"):              {},
			filepath.Join("1", "GPDB_6_301908232"): {Mode: os.ModeDir},
		}

		err := upgrade.VerifyTablespaceLocation(ts, "/filespace/demoDataDir0/16386")
		if err != nil {
			t.Errorf("unexpected error %+v", err)
		}
	})

	t.Run("errors when failing to read tablespace directory", func(t *testing.T) {
		ts := fstest.MapFS{
			filepath.Join("1", "GPDB_6_301908232"): {Mode: os.ModeDir},
		}

		utils.System.ReadDir = func(fsys fs.FS, name string) ([]fs.DirEntry, error) {
			return nil, os.ErrPermission
		}
		defer func() {
			utils.System.ReadDir = fs.ReadDir
		}()

		err := upgrade.VerifyTablespaceLocation(ts, "/filespace/demoDataDir0/16386")
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("got error %#v want %#v", err, os.ErrPermission)
		}
	})

	t.Run("errors when there are no tablespace directories", func(t *testing.T) {
		ts := fstest.MapFS{}
		tsLocation := "/filespace/demoDataDir0/16386"

		err := upgrade.VerifyTablespaceLocation(ts, tsLocation)
		expected := fmt.Sprintf("invalid tablespace location %q", tsLocation)
		if err.Error() != expected {
			t.Errorf("got error %#v want %#v", err, expected)
		}
	})

	t.Run("errors when failing to verify legacy tablespace directory", func(t *testing.T) {
		ts := fstest.MapFS{
			filepath.Join("12094", upgrade.PGVersion): {},
			filepath.Join("12094", "16384"):           {},
		}

		utils.System.StatFS = func(fsys fs.FS, name string) (fs.FileInfo, error) {
			return nil, os.ErrPermission
		}
		defer func() {
			utils.System.StatFS = fs.Stat
		}()

		err := upgrade.VerifyTablespaceLocation(ts, "/filespace/demoDataDir0/16386")
		expected := xerrors.Errorf("checking path exists for legacy tablespace dbOID directory %q: %w", filepath.Join("12094", upgrade.PGVersion), os.ErrPermission)
		if err.Error() != expected.Error() {
			t.Errorf("got error %#v want %#v", err, expected)
		}
	})

	t.Run("errors when failing to verify non-legacy tablespace directory", func(t *testing.T) {
		ts := fstest.MapFS{
			filepath.Join("1", "non_tablespace_directory"): {Mode: os.ModeDir},
		}

		err := upgrade.VerifyTablespaceLocation(ts, "/filespace/demoDataDir0/16386")
		expected := xerrors.Errorf(`Invalid tablespace directory. Expected "1/non_tablespace_directory" to start with "GPDB_".`)
		if err.Error() != expected.Error() {
			t.Errorf("got error %#v want %#v", err, expected)
		}
	})

	t.Run("errors when directory is not a legacy and non-legacy tablespace directory", func(t *testing.T) {
		ts := fstest.MapFS{
			filepath.Join("1"): {Mode: os.ModeDir},
		}

		err := upgrade.VerifyTablespaceLocation(ts, "/filespace/demoDataDir0/16386")
		expected := xerrors.Errorf(`invalid tablespace directory "/filespace/demoDataDir0/16386/1"`)
		if err.Error() != expected.Error() {
			t.Errorf("got error %#v want %#v", err, expected)
		}
	})
}

func TestVerifyLegacyTablespaceDirectory(t *testing.T) {
	t.Run("succeeds for legacy tablespace directories", func(t *testing.T) {
		ts := fstest.MapFS{
			filepath.Join("12094", upgrade.PGVersion): {},
			filepath.Join("12094", "16384"):           {},
		}

		exists, err := upgrade.VerifyLegacyTablespaceDbOIDDirectory(ts, "12094")
		if err != nil {
			t.Errorf("unexpected error %+v", err)
		}

		if !exists {
			t.Error(`expected path "12094" to exist`)
		}
	})

	t.Run("returns false for non-legacy tablespace directories", func(t *testing.T) {
		ts := fstest.MapFS{
			filepath.Join("1", "GPDB_6_301908232"): {Mode: os.ModeDir},
		}

		exists, err := upgrade.VerifyLegacyTablespaceDbOIDDirectory(ts, "1")
		if err != nil {
			t.Errorf("unexpected error %+v", err)
		}

		if exists {
			t.Error(`expected path "1/PG_VERSION" to not exist`)
		}
	})

	t.Run("returns false when path does not exist", func(t *testing.T) {
		ts := fstest.MapFS{}
		exists, err := upgrade.VerifyLegacyTablespaceDbOIDDirectory(ts, "12094")
		if err != nil {
			t.Errorf("unexpected error %+v", err)
		}

		if exists {
			t.Error(`expected path "12094/PG_VERSION" to not exist`)
		}
	})

	t.Run("errors when failing to check if path exists", func(t *testing.T) {
		utils.System.StatFS = func(fsys fs.FS, name string) (fs.FileInfo, error) {
			return nil, os.ErrPermission
		}
		defer func() {
			utils.System.StatFS = fs.Stat
		}()

		exists, err := upgrade.VerifyLegacyTablespaceDbOIDDirectory(fstest.MapFS{}, "12094")
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("got error %#v want %#v", err, os.ErrPermission)
		}

		if exists {
			t.Error("expected path to not exist")
		}
	})
}

func TestVerifyTablespaceDirectory(t *testing.T) {
	t.Run("succeeds for tablespace directories", func(t *testing.T) {
		ts := fstest.MapFS{
			filepath.Join("1", "GPDB_6_301908232"): {Mode: os.ModeDir},
		}

		exists, err := upgrade.VerifyTablespaceDbIDDirectory(ts, "1")
		if err != nil {
			t.Errorf("unexpected error %+v", err)
		}

		if !exists {
			t.Error(`expected path "1" to exist`)
		}
	})

	t.Run("fails with legacy tablespace directories", func(t *testing.T) {
		ts := fstest.MapFS{
			filepath.Join("12094", upgrade.PGVersion): {},
			filepath.Join("12094", "16384"):           {},
		}

		exists, err := upgrade.VerifyTablespaceDbIDDirectory(ts, "12094")
		if err != nil {
			t.Errorf("unexpected error %+v", err)
		}

		if exists {
			t.Error(`expected path "12094/16384" not exist`)
		}
	})

	t.Run("fails if path does not look like tablespace directory", func(t *testing.T) {
		ts := fstest.MapFS{
			filepath.Join("1", "non_tablespace_directory"): {Mode: os.ModeDir},
		}

		exists, err := upgrade.VerifyTablespaceDbIDDirectory(ts, "1")
		expected := `Invalid tablespace directory. Expected "1/non_tablespace_directory" to start with "GPDB_".`
		if err.Error() != expected {
			t.Errorf("got error %#v want %#v", err, expected)
		}

		if exists {
			t.Error(`expected path "1/non_tablespace_directory" not exist`)
		}
	})

	t.Run("errors when failing to read directory", func(t *testing.T) {
		exists, err := upgrade.VerifyTablespaceDbIDDirectory(fstest.MapFS{}, "1")
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("got error %#v want %#v", err, os.ErrNotExist)
		}

		if exists {
			t.Error(`expected path "1" not exist`)
		}
	})
}
