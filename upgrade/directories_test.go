// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package upgrade_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func TestTempDataDir(t *testing.T) {
	var id upgrade.ID

	cases := []struct {
		datadir        string
		segPrefix      string
		expectedFormat string // %s will be replaced with id.String()
	}{
		{"/data/seg-1", "seg", "/data/seg.%s.-1"},
		{"/data/master/gpseg-1", "gpseg", "/data/master/gpseg.%s.-1"},
		{"/data/seg1", "seg", "/data/seg.%s.1"},
		{"/data/seg1/", "seg", "/data/seg.%s.1"},
		{"/data/standby", "seg", "/data/standby.%s"},
	}

	for _, c := range cases {
		actual := upgrade.TempDataDir(c.datadir, c.segPrefix, id)
		expected := fmt.Sprintf(c.expectedFormat, id)

		if actual != expected {
			t.Errorf("TempDataDir(%q, %q, id) = %q, want %q",
				c.datadir, c.segPrefix, actual, expected)
		}
	}
}

func ExampleTempDataDir() {
	var id upgrade.ID

	master := upgrade.TempDataDir("/data/master/seg-1", "seg", id)
	standby := upgrade.TempDataDir("/data/standby", "seg", id)
	segment := upgrade.TempDataDir("/data/primary/seg3", "seg", id)

	fmt.Println(master)
	fmt.Println(standby)
	fmt.Println(segment)
	// Output:
	// /data/master/seg.AAAAAAAAAAA.-1
	// /data/standby.AAAAAAAAAAA
	// /data/primary/seg.AAAAAAAAAAA.3
}

func TestGetArchiveDirectoryName(t *testing.T) {
	// Make sure every part of the date is distinct, to catch mistakes in
	// formatting (e.g. using seconds rather than minutes).
	stamp := time.Date(2000, 03, 14, 12, 15, 45, 1, time.Local)

	var id upgrade.ID
	actual := upgrade.GetArchiveDirectoryName(id, stamp)

	expected := fmt.Sprintf("gpupgrade-%s-2000-03-14T12:15", id.String())
	if actual != expected {
		t.Errorf("GetArchiveDirectoryName() = %q, want %q", actual, expected)
	}
}

func TestArchiveSource(t *testing.T) {
	_, _, log := testlog.SetupLogger()

	t.Run("successfully renames source to archive, and target to source", func(t *testing.T) {
		source, target, cleanup := testutils.MustCreateDataDirs(t)
		defer cleanup(t)

		err := upgrade.RenameDirectories(source, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		testutils.VerifyRename(t, source, target)
	})

	t.Run("returns early if already renamed", func(t *testing.T) {
		source, target, cleanup := testutils.MustCreateDataDirs(t)
		defer cleanup(t)

		// To return early create archive directory
		archive := target + upgrade.OldSuffix
		err := os.Rename(target, archive)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		called := false
		utils.System.Rename = func(old, new string) error {
			called = true
			return nil
		}
		defer func() {
			utils.System.Rename = os.Rename
		}()

		testutils.VerifyRename(t, source, target)

		err = upgrade.RenameDirectories(source, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		if called {
			t.Errorf("expected rename to not be called")
		}
	})

	t.Run("bubbles up errors", func(t *testing.T) {
		source, target, cleanup := testutils.MustCreateDataDirs(t)
		defer cleanup(t)

		expected := errors.New("permission denied")
		utils.System.Rename = func(old, new string) error {
			return expected
		}
		defer func() {
			utils.System.Rename = os.Rename
		}()

		err := upgrade.RenameDirectories(source, target)
		if !errors.Is(err, expected) {
			t.Errorf("got %#v want %#v", err, expected)
		}
	})

	t.Run("errors when renaming a directory that is not like postgres", func(t *testing.T) {
		source := testutils.GetTempDir(t, "source")
		defer testutils.MustRemoveAll(t, source)

		target := testutils.GetTempDir(t, "target")
		defer testutils.MustRemoveAll(t, target)

		err := upgrade.RenameDirectories(source, target)

		var errs errorlist.Errors
		if !errors.As(err, &errs) {
			t.Fatalf("returned %#v want error type %T", err, errs)
		}

		for _, err := range errs {
			expected := upgrade.ErrInvalidDataDirectory
			if !errors.Is(err, expected) {
				t.Errorf("returned error %#v want %#v", err, expected)
			}
		}
	})

	t.Run("only renames target when renameDirectory is OnlyRenameTarget", func(t *testing.T) {
		source, target, cleanup := testutils.MustCreateDataDirs(t)
		defer cleanup(t)
		testutils.MustRemoveAll(t, source) // source does not exist when only renaming the target

		calls := 0
		utils.System.Rename = func(old, new string) error {
			calls++

			if old != target {
				t.Errorf("got %q want %q", old, target)
			}

			if new != source {
				t.Errorf("got %q want %q", new, source)
			}

			return os.Rename(old, new)
		}
		defer func() {
			utils.System.Rename = os.Rename
		}()

		err := upgrade.RenameDirectories(source, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		if calls != 1 {
			t.Errorf("expected rename to be called once")
		}

		testutils.PathMustExist(t, source)
		testutils.PathMustNotExist(t, target)
	})

	t.Run("when renaming succeeds then a re-run succeeds", func(t *testing.T) {
		source, target, cleanup := testutils.MustCreateDataDirs(t)
		defer cleanup(t)

		err := upgrade.RenameDirectories(source, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		testutils.VerifyRename(t, source, target)

		err = upgrade.RenameDirectories(source, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		testutils.VerifyRename(t, source, target)

		testlog.VerifyLogDoesNotContain(t, log, "Source directory does not exist")
	})

	t.Run("when renaming the source fails then a re-run succeeds", func(t *testing.T) {
		source, target, cleanup := testutils.MustCreateDataDirs(t)
		defer cleanup(t)

		expected := errors.New("permission denied")
		utils.System.Rename = func(old, new string) error {
			if old == source {
				return expected
			}
			return os.Rename(old, new)
		}

		err := upgrade.RenameDirectories(source, target)
		if !errors.Is(err, expected) {
			t.Errorf("got %#v want %#v", err, expected)
		}

		archive := target + upgrade.OldSuffix

		testutils.PathMustExist(t, source)
		testutils.PathMustNotExist(t, archive)
		testutils.PathMustExist(t, target)

		utils.System.Rename = os.Rename

		err = upgrade.RenameDirectories(source, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		testutils.VerifyRename(t, source, target)

		testlog.VerifyLogDoesNotContain(t, log, "Source directory does not exist")
	})

	t.Run("when renaming the target fails then a re-run succeeds", func(t *testing.T) {
		source, target, cleanup := testutils.MustCreateDataDirs(t)
		defer cleanup(t)

		expected := errors.New("permission denied")
		utils.System.Rename = func(old, new string) error {
			if old == target {
				return expected
			}
			return os.Rename(old, new)
		}

		err := upgrade.RenameDirectories(source, target)
		if !errors.Is(err, expected) {
			t.Errorf("got %#v want %#v", err, expected)
		}

		archive := target + upgrade.OldSuffix

		testutils.PathMustNotExist(t, source)
		testutils.PathMustExist(t, archive)
		testutils.PathMustExist(t, target)

		utils.System.Rename = os.Rename

		err = upgrade.RenameDirectories(source, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		testutils.VerifyRename(t, source, target)

		testlog.VerifyLogContains(t, log, "Source directory not found")
	})
}

func setup(t *testing.T) (teardown func(), directories []string, requiredPaths []string) {
	requiredPaths = []string{"pg_file1", "pg_file2"}
	var dataDirectories = []string{"/data/dbfast_mirror1/seg1", "/data/dbfast_mirror2/seg2"}
	rootDir, directories := setupDirs(t, dataDirectories, requiredPaths)
	teardown = func() {
		err := os.RemoveAll(rootDir)
		if err != nil {
			t.Fatalf("error %#v when deleting directory %#v", err, rootDir)
		}
	}

	return teardown, directories, requiredPaths
}

func TestDeleteDirectories(t *testing.T) {
	testlog.SetupLogger()

	utils.System.Hostname = func() (string, error) {
		return "localhost.local", nil
	}
	defer func() {
		utils.System.Hostname = os.Hostname
	}()

	t.Run("successfully deletes the directories if all required paths exist in that directory", func(t *testing.T) {
		var buf bytes.Buffer
		devNull := testutils.DevNullSpy{
			OutStream: &buf,
		}
		teardown, directories, requiredPaths := setup(t)
		defer teardown()

		err := upgrade.DeleteDirectories(directories, requiredPaths, devNull)

		if err != nil {
			t.Errorf("unexpected error got %+v", err)
		}

		for _, dataDir := range directories {
			if _, err := os.Stat(dataDir); err == nil {
				t.Errorf("dataDir %s exists", dataDir)
			}
		}

		expected := regexp.MustCompile(`Deleting directory: ".*/data/dbfast_mirror1/seg1" on host "localhost.local"\nDeleting directory: ".*/data/dbfast_mirror2/seg2" on host "localhost.local"`)

		actual := buf.String()
		if !expected.MatchString(actual) {
			t.Errorf("got stream output %s want %s", actual, expected)
		}
	})

	t.Run("rerun after a previous successfully execution must succeed", func(t *testing.T) {
		teardown, directories, requiredPaths := setup(t)
		defer teardown()

		err := upgrade.DeleteDirectories(directories, requiredPaths, step.DevNullStream)

		if err != nil {
			t.Errorf("unexpected error got %+v", err)
		}

		for _, dataDir := range directories {
			if _, err := os.Stat(dataDir); err == nil {
				t.Errorf("dataDir %s exists", dataDir)
			}
		}

		err = upgrade.DeleteDirectories(directories, requiredPaths, step.DevNullStream)

		if err != nil {
			t.Errorf("unexpected error during rerun, got %+v", err)
		}
	})

	t.Run("fails when the required paths are not in the directories", func(t *testing.T) {
		teardown, directories, _ := setup(t)
		defer teardown()

		err := upgrade.DeleteDirectories(directories, []string{"a", "b"}, step.DevNullStream)

		var errs errorlist.Errors
		if !errors.As(err, &errs) {
			t.Fatalf("got error %#v, want type %T", err, errs)
		}

		if len(errs) != 4 {
			t.Errorf("received %d errors, want %d", len(errs), 4)
		}

		for _, err := range errs {
			if !errors.Is(err, os.ErrNotExist) {
				t.Errorf("got error %#v, want %#v", err, os.ErrNotExist)
			}
		}
	})

	t.Run("fails to remove one segment data directory", func(t *testing.T) {
		teardown, directories, requiredPaths := setup(t)
		defer teardown()

		fileToRemove := filepath.Join(directories[0], requiredPaths[0])
		if err := os.Remove(fileToRemove); err != nil {
			t.Errorf("unexpected error %+v", err)
		}

		err := upgrade.DeleteDirectories(directories, requiredPaths, step.DevNullStream)

		var actualErr *os.PathError
		if !errors.As(err, &actualErr) {
			t.Errorf("got error %#v, want %#v", err, "PathError")
		}

		if _, err := os.Stat(directories[0]); err != nil {
			t.Errorf("dataDir should exist, stat error %+v", err)
		}

		if _, err := os.Stat(directories[1]); err == nil {
			t.Errorf("dataDir %s exists", directories[1])
		}
	})

	t.Run("errors when hostname fails", func(t *testing.T) {
		teardown, directories, requiredPaths := setup(t)
		defer teardown()

		expected := errors.New("unable to resolve host name")
		utils.System.Hostname = func() (string, error) {
			return "", expected
		}
		defer func() {
			utils.System.Hostname = os.Hostname
		}()

		err := upgrade.DeleteDirectories(directories, requiredPaths, step.DevNullStream)
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v want %#v", err, expected)
		}
	})
}

func TestTablespacePath(t *testing.T) {
	t.Run("returns correct path", func(t *testing.T) {
		path := upgrade.TablespacePath("/tmp/testfs/master/demoDataDir-1/16386", 1, 6, "301908232")
		expected := "/tmp/testfs/master/demoDataDir-1/16386/1/GPDB_6_301908232"
		if path != expected {
			t.Errorf("got %q want %q", path, expected)
		}
	})
}

func TestPathExist(t *testing.T) {
	t.Run("path exists", func(t *testing.T) {
		dir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir)

		doesExist, err := upgrade.PathExist(dir)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}

		if !doesExist {
			t.Errorf("expected path %q to exist", dir)
		}
	})

	t.Run("path does not exists", func(t *testing.T) {
		dir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir)

		path := filepath.Join(dir, "doesnotexist")
		doesExist, err := upgrade.PathExist(path)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}

		if doesExist {
			t.Errorf("expected path %q to not exist", dir)
		}
	})

	t.Run("returns error", func(t *testing.T) {
		expected := os.ErrInvalid
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			return nil, expected
		}
		defer utils.ResetSystemFunctions()

		doesExist, err := upgrade.PathExist("somepath")
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v want %#v", err, expected)
		}

		if doesExist {
			t.Error("expected path to not exist")
		}
	})
}

func setupDirs(t *testing.T, subdirectories []string, requiredPaths []string) (tmpDir string, createdDirectories []string) {
	var err error
	tmpDir, err = ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("error creating temporary directory: %v", err)
	}

	for _, dir := range subdirectories {
		createdDirectories = append(createdDirectories, createDataDir(t, dir, tmpDir, requiredPaths))
	}

	return tmpDir, createdDirectories
}

func createDataDir(t *testing.T, name, tmpDir string, requiredPaths []string) (dirPath string) {
	dirPath = filepath.Join(tmpDir, name)

	err := os.MkdirAll(dirPath, userRWX)
	if err != nil {
		t.Errorf("error creating path: %v", err)
	}

	for _, fileName := range requiredPaths {
		filePath := filepath.Join(dirPath, fileName)
		testutils.MustWriteToFile(t, filePath, "")
	}

	return dirPath
}
