package utils_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestMove(t *testing.T) {
	t.Run("move run successfully", func(t *testing.T) {
		src := testutils.GetTempDir(t, "")
		defer os.RemoveAll(src)

		file := "file.txt"
		srcFile := filepath.Join(src, file)
		_, err := os.Create(srcFile)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}

		dst := src + "-aftermove"
		err = utils.Move(src, dst)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}
		defer os.RemoveAll(dst)

		dstFile := filepath.Join(dst, file)
		_, err = os.Stat(dstFile)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}

		_, err = os.Stat(src)
		if !os.IsNotExist(err) {
			t.Errorf("expected source directory to not exist: %#v", err)
		}
	})

	t.Run("move fails", func(t *testing.T) {
		src := ""
		dst := testutils.GetTempDir(t, "")
		defer os.RemoveAll(dst)

		err := utils.Move(src, dst)
		if err == nil {
			t.Errorf("expected error")
		}

		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Errorf("got %T, want %T", err, exitError)
		}
	})
}

func TestAtomicallyWrite(t *testing.T) {
	t.Run("successfully writes", func(t *testing.T) {
		dir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir)

		path := filepath.Join(dir, upgrade.ConfigFileName)

		expected := "testing writing to a file"
		if err := utils.AtomicallyWrite(path, []byte(expected)); err != nil {
			t.Errorf("AtomicallyWrite returned error %+v", err)
		}

		contents := testutils.MustReadFile(t, path)
		if contents != expected {
			t.Errorf("wrote %#q want %q", contents, expected)
		}
	})

	t.Run("errors when directory does not exist", func(t *testing.T) {
		path := "/does/not/exist"

		err := utils.AtomicallyWrite(path, []byte{})
		var expected *os.PathError
		if !errors.As(err, &expected) {
			t.Errorf("returned error type %T want %T", err, expected)
		}

		if upgrade.PathExists(path) {
			t.Errorf("expected file %q to not exist", path)
		}
	})
}
