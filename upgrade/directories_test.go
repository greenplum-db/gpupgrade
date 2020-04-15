package upgrade_test

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
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

func TestRenameDataDirectory(t *testing.T) {
	testhelper.SetupTestLogger()

	t.Run("successfully renames source to archive, and target to source", func(t *testing.T) {
		source, archive, target, cleanup := mustCreateDirs(t)
		defer cleanup(t)

		err := upgrade.RenameDataDirectory(source, archive, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		verifyRename(t, source, archive, target)
	})

	t.Run("returns early if already renamed", func(t *testing.T) {
		source := testutils.GetTempDir(t, "source")
		defer os.RemoveAll(source)

		archive := testutils.GetTempDir(t, "archive")
		defer os.RemoveAll(archive)

		target := ""

		called := false
		utils.System.Rename = func(old, new string) error {
			called = true
			return nil
		}
		defer func() {
			utils.System.Rename = os.Rename
		}()

		verifyRename(t, source, archive, target)

		err := upgrade.RenameDataDirectory(source, archive, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		if called {
			t.Errorf("expected rename to not be called")
		}
	})

	t.Run("bubbles up errors", func(t *testing.T) {
		source, archive, target, cleanup := mustCreateDirs(t)
		defer cleanup(t)

		expected := errors.New("permission denied")
		utils.System.Rename = func(old, new string) error {
			return expected
		}
		defer func() {
			utils.System.Rename = os.Rename
		}()

		err := upgrade.RenameDataDirectory(source, archive, target)
		if !xerrors.Is(err, expected) {
			t.Errorf("got %#v want %#v", err, expected)
		}
	})

	t.Run("it returns other LinkErrors when renaming the source fails for errors other than ENOENT", func(t *testing.T) {
		source, archive, target, cleanup := mustCreateDirs(t)
		defer cleanup(t)

		expected := &os.LinkError{Err: syscall.EEXIST}
		utils.System.Rename = func(old, new string) error {
			if old == source {
				return expected
			}
			return os.Rename(old, new)
		}
		defer func() {
			utils.System.Rename = os.Rename
		}()

		err := upgrade.RenameDataDirectory(source, archive, target)
		if !xerrors.Is(err, expected) {
			t.Errorf("got %#v want %#v", err, expected)
		}
	})

	t.Run("when renaming succeeds then a re-run succeeds", func(t *testing.T) {
		source, archive, target, cleanup := mustCreateDirs(t)
		defer cleanup(t)

		err := upgrade.RenameDataDirectory(source, archive, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		verifyRename(t, source, archive, target)

		err = upgrade.RenameDataDirectory(source, archive, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		verifyRename(t, source, archive, target)
	})

	t.Run("when renaming the source fails then a re-run succeeds", func(t *testing.T) {
		source, archive, target, cleanup := mustCreateDirs(t)
		defer cleanup(t)

		expected := errors.New("permission denied")
		utils.System.Rename = func(old, new string) error {
			if old == source {
				return expected
			}
			return os.Rename(old, new)
		}

		err := upgrade.RenameDataDirectory(source, archive, target)
		if !xerrors.Is(err, expected) {
			t.Errorf("got %#v want %#v", err, expected)
		}

		if !upgrade.PathExists(source) {
			t.Errorf("expected source %q to exist", source)
		}

		if upgrade.PathExists(archive) {
			t.Errorf("expected archive %q to not exist", archive)
		}

		if !upgrade.PathExists(target) {
			t.Errorf("expected target %q to exist", target)
		}

		utils.System.Rename = os.Rename

		err = upgrade.RenameDataDirectory(source, archive, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		verifyRename(t, source, archive, target)
	})

	t.Run("when renaming the target fails then a re-run succeeds", func(t *testing.T) {
		source, archive, target, cleanup := mustCreateDirs(t)
		defer cleanup(t)

		expected := errors.New("permission denied")
		utils.System.Rename = func(old, new string) error {
			if old == target {
				return expected
			}
			return os.Rename(old, new)
		}

		err := upgrade.RenameDataDirectory(source, archive, target)
		if !xerrors.Is(err, expected) {
			t.Errorf("got %#v want %#v", err, expected)
		}

		if upgrade.PathExists(source) {
			t.Errorf("expected source %q to not exist", source)
		}

		if !upgrade.PathExists(archive) {
			t.Errorf("expected archive %q to exist", archive)
		}

		if !upgrade.PathExists(target) {
			t.Errorf("expected target %q to exist", target)
		}

		utils.System.Rename = os.Rename

		err = upgrade.RenameDataDirectory(source, archive, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		verifyRename(t, source, archive, target)
	})
}

func mustCreateDirs(t *testing.T) (string, string, string, func(*testing.T)) {
	t.Helper()

	source := testutils.GetTempDir(t, "source")
	archive := source + upgrade.OldSuffix
	target := testutils.GetTempDir(t, "target")

	return source, archive, target, func(t *testing.T) {
		if err := os.RemoveAll(source); err != nil {
			t.Errorf("removing source directory: %v", err)
		}
		if err := os.RemoveAll(archive); err != nil {
			t.Errorf("removing archive directory: %v", err)
		}
		if err := os.RemoveAll(target); err != nil {
			t.Errorf("removing target directory: %v", err)
		}
	}
}

func verifyRename(t *testing.T, source, archive, target string) {
	t.Helper()

	if !upgrade.PathExists(source) {
		t.Errorf("expected source %q to exist", source)
	}

	if !upgrade.PathExists(archive) {
		t.Errorf("expected archive %q to exist", archive)
	}

	if upgrade.PathExists(target) {
		t.Errorf("expected target %q to not exist", target)
	}
}
