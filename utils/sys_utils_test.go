package utils_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/testutils"
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
		if !xerrors.As(err, &exitError) {
			t.Errorf("got %T, want %T", err, exitError)
		}
	})
}
