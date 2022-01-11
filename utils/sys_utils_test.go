package utils_test

import (
	"errors"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestJSONFile(t *testing.T) {
	stateDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(stateDir); err != nil {
			t.Errorf("removing temp directory: %v", err)
		}
	}()

	JSONFileName := "file.json"
	path := filepath.Join(stateDir, JSONFileName)

	t.Run("creates JSON file if it does not exist", func(t *testing.T) {
		_, err := os.Open(path)
		if !os.IsNotExist(err) {
			t.Errorf("returned error %#v want ErrNotExist", err)
		}

		statusFile, err := utils.GetJSONFile(stateDir, JSONFileName)
		if err != nil {
			t.Errorf("unexpected error %v", err)
		}

		contents := testutils.MustReadFile(t, statusFile)
		if contents != "{}" {
			t.Errorf("read %q want {}", contents)
		}
	})

	t.Run("does not create JSON file if it already exists", func(t *testing.T) {
		expected := "1234"
		testutils.MustWriteToFile(t, path, expected)

		statusFile, err := utils.GetJSONFile(stateDir, JSONFileName)
		if err != nil {
			t.Errorf("unexpected error %v", err)
		}

		contents := testutils.MustReadFile(t, statusFile)
		if contents != expected {
			t.Errorf("read %q want %q", contents, expected)
		}
	})
}

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

		testutils.PathMustNotExist(t, path)
	})
}

func TestRemoveDuplicates(t *testing.T) {
	t.Run("removes duplicates while preserving order", func(t *testing.T) {
		cases := []struct {
			input    []string
			expected []string
		}{
			{
				input:    []string{"1", "2", "3"},
				expected: []string{"1", "2", "3"},
			},
			{
				input:    []string{"1", "2", "3", "2"},
				expected: []string{"1", "2", "3"},
			},
			{
				input:    []string{"1", "2", "3", "3", "3", "2", "1"},
				expected: []string{"1", "2", "3"},
			},
		}

		for _, c := range cases {
			actual := utils.RemoveDuplicates(c.input)

			expected := []string{"1", "2", "3"}
			if !reflect.DeepEqual(actual, expected) {
				t.Errorf("got %q, want %q", actual, expected)
			}
		}
	})
}
