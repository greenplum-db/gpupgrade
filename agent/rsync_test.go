package agent_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/agent"
)

func getTempDir(t *testing.T) string {
	sourceDir, err := ioutil.TempDir("", "rsync-source")
	if err != nil {
		t.Fatalf("creating temporary directory: %+v", err)
	}

	return sourceDir
}

func writeToFile(filepath string, contents []byte, t *testing.T) {
	err := ioutil.WriteFile(filepath, contents, 0644)

	if err != nil {
		t.Fatalf("error writing file '%v'", filepath)
	}
}

func TestRsync(t *testing.T) {
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skipf("tests require rsync (%v)", err)
	}

	// These are "live" integration tests. Plug exec.Command back into the
	// system.
	agent.SetRsyncCommand(exec.Command)
	defer func() { agent.SetRsyncCommand(nil) }()

	t.Run("it copies data from a source directory to a target directory", func(t *testing.T) {
		sourceDir := getTempDir(t)
		defer os.RemoveAll(sourceDir)

		targetDir := getTempDir(t)
		defer os.RemoveAll(targetDir)

		writeToFile(filepath.Join(sourceDir, "hi"), []byte("hi"), t)

		if err := agent.Rsync(sourceDir, targetDir, []string{}); err != nil {
			t.Errorf("Rsync() returned error %+v", err)
		}

		targetContents, _ := ioutil.ReadFile(filepath.Join(targetDir, "/hi"))

		if !bytes.Equal(targetContents, []byte("hi")) {
			t.Errorf("target directory file 'hi' contained %v, wanted %v",
				targetContents,
				"hi")
		}
	})

	t.Run("it removes files that existed in the target directory before the sync", func(t *testing.T) {
		sourceDir := getTempDir(t)
		defer os.RemoveAll(sourceDir)

		targetDir := getTempDir(t)
		defer os.RemoveAll(targetDir)

		writeToFile(filepath.Join(targetDir, "target-file-that-should-get-removed"), []byte("goodbye"), t)

		if err := agent.Rsync(sourceDir, targetDir, []string{}); err != nil {
			t.Errorf("Rsync() returned error %+v", err)
		}

		_, statError := os.Stat(filepath.Join(targetDir, "target-file-that-should-get-removed"))

		if os.IsExist(statError) {
			t.Errorf("target directory file 'target-file-that-should-get-removed' should not exist, but it does")
		}
	})

	t.Run("it does not copy files from the source directory when in the exclusion list", func(t *testing.T) {
		sourceDir := getTempDir(t)
		defer os.RemoveAll(sourceDir)

		targetDir := getTempDir(t)
		defer os.RemoveAll(targetDir)

		writeToFile(filepath.Join(sourceDir, "source-file-that-should-get-excluded"), []byte("goodbye"), t)

		err := agent.Rsync(sourceDir, targetDir, []string{"source-file-that-should-get-excluded"})
		if err != nil {
			t.Errorf("Rsync() returned error %+v", err)
		}

		_, statError := os.Stat(filepath.Join(targetDir, "source-file-that-should-get-excluded"))

		if os.IsExist(statError) {
			t.Errorf("target directory file 'source-file-that-should-get-excluded' should not exist, but it does")
		}
	})

	t.Run("it preserves files in the target directory when in the exclusion list", func(t *testing.T) {
		sourceDir := getTempDir(t)
		defer os.RemoveAll(sourceDir)

		targetDir := getTempDir(t)
		defer os.RemoveAll(targetDir)

		writeToFile(filepath.Join(sourceDir, "source-file-that-should-get-copied"), []byte("new file"), t)
		writeToFile(filepath.Join(targetDir, "target-file-that-should-get-ignored"), []byte("i'm still here"), t)
		writeToFile(filepath.Join(targetDir, "another-target-file-that-should-get-ignored"), []byte("i'm still here"), t)

		err := agent.Rsync(sourceDir, targetDir, []string{"target-file-that-should-get-ignored", "another-target-file-that-should-get-ignored"})
		if err != nil {
			t.Errorf("Rsync() returned error %+v", err)
		}

		_, statError := os.Stat(filepath.Join(targetDir, "target-file-that-should-get-ignored"))

		if os.IsNotExist(statError) {
			t.Error("target directory file 'target-file-that-should-get-ignored' should still exist, but it does not")
		}

		_, statError = os.Stat(filepath.Join(targetDir, "another-target-file-that-should-get-ignored"))

		if os.IsNotExist(statError) {
			t.Error("target directory file 'another-target-file-that-should-get-ignored' should still exist, but it does not")
		}

		_, statError = os.Stat(filepath.Join(targetDir, "source-file-that-should-get-copied"))

		if os.IsNotExist(statError) {
			t.Error("target directory file 'source-file-that-should-get-copied' should exist, but does not")
		}
	})

	t.Run("it bubbles up exec.ExitError errors as rsync errors", func(t *testing.T) {
		sourceDir := getTempDir(t)
		defer os.RemoveAll(sourceDir)

		targetDir := "/tmp/some/invalid/target/dir"
		defer os.RemoveAll(targetDir)

		writeToFile(filepath.Join(sourceDir, "some-file"), []byte("hi"), t)

		err := agent.Rsync(sourceDir, targetDir, []string{""})

		var rsyncError agent.RsyncError

		if !xerrors.As(err, &rsyncError) {
			t.Errorf("got error %#v, wanted type %T", err, rsyncError)
		}

		if !strings.Contains(rsyncError.Error(), "rsync: mkdir \"/tmp/some/invalid/target/dir\" failed") {
			t.Errorf("got %v, wanted rsync error 'rsync failed cause I said so'",
				err.Error())
		}
	})

	t.Run("it bubbles up exec.Error errors as rsync errors", func(t *testing.T) {
		originalPath := destroyPath()
		defer restorePath(originalPath)

		sourceDir := getTempDir(t)
		defer os.RemoveAll(sourceDir)

		targetDir := "/tmp/some/invalid/target/dir"
		defer os.RemoveAll(targetDir)

		writeToFile(filepath.Join(sourceDir, "some-file"), []byte("hi"), t)

		err := agent.Rsync(sourceDir, targetDir, []string{""})

		var rsyncError agent.RsyncError

		if !xerrors.As(err, &rsyncError) {
			t.Errorf("got error %#v, wanted type %T", err, rsyncError)
		}

		if !strings.Contains(rsyncError.Error(), "exec: \"rsync\": executable file not found in $PATH") {
			t.Errorf("got %v, wanted rsync error 'exit status 1'",
				err.Error())
		}
	})
}

func restorePath(originalPath string) {
	os.Setenv("PATH", originalPath)
}

func destroyPath() string {
	var originalPath = os.Getenv("PATH")

	os.Setenv("PATH", "/nothing")

	return originalPath
}
