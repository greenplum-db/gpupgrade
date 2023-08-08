// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package testutils

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

func RemoteProcessMustBeRunning(t *testing.T, host string, process string) {
	t.Helper()

	isRunning, err := checkRemoteProcess(t, host, process)
	if err != nil {
		t.Fatalf("unexpected err: %#v", err)
	}

	if !isRunning {
		t.Fatalf("expected %q to be running", process)
	}
}

func RemoteProcessMustNotBeRunning(t *testing.T, host string, process string) {
	t.Helper()

	isRunning, err := checkRemoteProcess(t, host, process)
	if err != nil {
		t.Fatalf("unexpected err: %#v", err)
	}

	if isRunning {
		t.Fatalf("expected %q to not be running", process)
	}
}

func checkRemoteProcess(t *testing.T, host string, process string) (bool, error) {
	t.Helper()

	cmd := exec.Command("ssh", host, "bash", "-c", fmt.Sprintf(`'pgrep -f %q'`, process))
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() == 1 {
			// No processes were matched
			return false, nil
		}
	}

	if err != nil {
		return false, err
	}

	return true, nil
}

func RemotePathMustExist(t *testing.T, host string, path string) {
	t.Helper()
	checkRemotePath(t, host, path, true)
}

func RemotePathMustNotExist(t *testing.T, host string, path string) {
	t.Helper()
	checkRemotePath(t, host, path, false)
}

func checkRemotePath(t *testing.T, host string, path string, shouldExist bool) {
	t.Helper()

	// Since path can be either a directory or file use -e instead of -d and -f.
	// To support glob matching (ie: *) use %s instead of %q.
	cmd := exec.Command("ssh", host, fmt.Sprintf("[ -e %s ]", path))
	output, err := cmd.CombinedOutput()
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		exists := exitError.ProcessState.ExitCode() == 0
		if shouldExist && !exists {
			t.Fatalf("expected path %q to exist on host %q", path, host)
		}

		if !shouldExist && exists {
			t.Fatalf("expected path %q to not exist on host %q", path, host)
		}
	}

	if err != nil && !errors.As(err, &exitError) {
		t.Fatalf("unexpected err: %v stderr: %q", err, output)
	}
}

// MustWriteToRemoteFile writes a local file in a temp directory and rsync's it
// to the remote host. It does this since writing remotely is difficult and
// error prone.
func MustWriteToRemoteFile(t *testing.T, host string, path string, contents string) {
	t.Helper()

	dir := GetTempDir(t, "")
	defer MustRemoveAll(t, dir)

	localPath := filepath.Join(dir, path)
	MustCreateDir(t, filepath.Dir(localPath))
	MustWriteToFile(t, localPath, contents)

	options := []rsync.Option{
		rsync.WithSources(localPath),
		rsync.WithDestinationHost(host),
		rsync.WithDestination(path),
		rsync.WithOptions(rsync.Options...),
		rsync.WithStream(step.DevNullStream),
	}
	err := rsync.Rsync(options...)
	if err != nil {
		t.Fatal(err)
	}
}

func MustRemoveAllRemotely(t *testing.T, host string, path string) {
	t.Helper()

	cmd := exec.Command("ssh", host, "rm", "-rf", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %v stderr: %q", err, output)
	}
}
func MustCreateDirRemotely(t *testing.T, host string, path string) {
	t.Helper()

	cmd := exec.Command("ssh", host, "mkdir", "-p", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %v stderr: %q", err, output)
	}
}

func MustMoveRemoteFile(t *testing.T, host string, source string, destination string) {
	t.Helper()

	cmd := exec.Command("ssh", host, "mv", source, destination)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %v stderr: %q", err, output)
	}
}
