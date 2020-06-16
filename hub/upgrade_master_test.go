// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

func Success() {}
func Failure() {
	os.Exit(1)
}

const StreamingMainStdout = "expected\nstdout\n"
const StreamingMainStderr = "process\nstderr\n"

// Streams the above stdout/err constants to the corresponding standard file
// descriptors, alternately interleaving five-byte chunks.
func StreamingMain() {
	stdout := bytes.NewBufferString(StreamingMainStdout)
	stderr := bytes.NewBufferString(StreamingMainStderr)

	for stdout.Len() > 0 || stderr.Len() > 0 {
		os.Stdout.Write(stdout.Next(5))
		os.Stderr.Write(stderr.Next(5))
	}
}

// Writes to stdout and ignores any failure to do so.
func BlindlyWritingMain() {
	// Ignore SIGPIPE. Note that the obvious signal.Ignore(syscall.SIGPIPE)
	// doesn't work as expected; see https://github.com/golang/go/issues/32386.
	signal.Notify(make(chan os.Signal, 1), syscall.SIGPIPE)

	fmt.Println("blah blah blah blah")
	fmt.Println("blah blah blah blah")
	fmt.Println("blah blah blah blah")
}

func init() {
	exectest.RegisterMains(
		Success,
		StreamingMain,
		BlindlyWritingMain,
		Failure,
	)
}

// Writes the current working directory to stdout.
func WorkingDirectoryMain() {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get working directory: %v", err)
		os.Exit(1)
	}

	fmt.Print(wd)
}

// Prints the environment, one variable per line, in NAME=VALUE format.
func EnvironmentMain() {
	for _, e := range os.Environ() {
		fmt.Println(e)
	}
}

func init() {
	exectest.RegisterMains(
		WorkingDirectoryMain,
		EnvironmentMain,
	)
}

func TestUpgradeMaster(t *testing.T) {
	source := MustCreateCluster(t, []greenplum.SegConfig{
		{ContentID: -1, Port: 5432, DataDir: "/data/old", DbID: 1, Role: "p"},
	})
	source.BinDir = "/old/bin"

	t.Run("masterSegmentFromCluster() creates a correct upgrade segment", func(t *testing.T) {
		seg := masterSegmentFromCluster(source)

		if seg.BinDir != source.BinDir {
			t.Errorf("BinDir was %q, want %q", seg.BinDir, source.BinDir)
		}
		if seg.DataDir != source.MasterDataDir() {
			t.Errorf("DataDir was %q, want %q", seg.DataDir, source.MasterDataDir())
		}
		if seg.DBID != source.GetDbidForContent(-1) {
			t.Errorf("DBID was %d, want %d", seg.DBID, source.GetDbidForContent(-1))
		}
		if seg.Port != source.MasterPort() {
			t.Errorf("Port was %d, want %d", seg.Port, source.MasterPort())
		}
	})

	// UpgradeMaster defers to upgrade.Run() for most of its work. Rather than
	// repeat those tests here, do some simple integration tests to verify that
	// output streams are hooked up correctly, then defer to the acceptance
	// tests for full end-to-end verification.

	target := MustCreateCluster(t, []greenplum.SegConfig{
		{ContentID: -1, Port: 5433, DataDir: "/data/new", DbID: 2, Role: "p"},
	})
	target.BinDir = "/new/bin"

	// We need a real temporary directory to change to. Replace MkdirAll() so
	// that we can make sure the directory is the correct one.
	tempDir, err := ioutil.TempDir("", "gpupgrade")
	if err != nil {
		t.Fatalf("creating temporary directory: %+v", err)
	}
	defer os.RemoveAll(tempDir)

	var createdWD string
	utils.System.MkdirAll = func(path string, perms os.FileMode) error {
		createdWD = path

		// Bail out if the implementation tries to touch any other directories.
		if !strings.HasPrefix(path, tempDir) {
			t.Fatalf("requested directory %q is not under temporary directory %q; refusing to create it",
				path, tempDir)
		}

		return os.MkdirAll(path, perms)
	}
	defer func() {
		utils.System = utils.InitializeSystemFunctions()
	}()

	t.Run("creates the desired working directory", func(t *testing.T) {
		SetExecCommand(exectest.NewCommand(Success))
		defer ResetExecCommand()

		SetRsyncExecCommand(exectest.NewCommand(Success))
		defer ResetRsyncExecCommand()

		err := UpgradeMaster(UpgradeMasterArgs{
			Source:      source,
			Target:      target,
			StateDir:    tempDir,
			Stream:      step.DevNullStream,
			CheckOnly:   false,
			UseLinkMode: false,
		})
		if err != nil {
			t.Errorf("returned error %+v", err)
		}

		expectedWD := upgrade.MasterWorkingDirectory(tempDir)
		if createdWD != expectedWD {
			t.Errorf("created working directory %q, want %q", createdWD, expectedWD)
		}
	})

	t.Run("streams stdout and stderr to the client", func(t *testing.T) {
		SetExecCommand(exectest.NewCommand(StreamingMain))
		defer ResetExecCommand()

		SetRsyncExecCommand(exectest.NewCommand(Success))
		defer ResetRsyncExecCommand()

		stream := new(step.BufferedStreams)

		err := UpgradeMaster(UpgradeMasterArgs{
			Source:      source,
			Target:      target,
			StateDir:    tempDir,
			Stream:      stream,
			CheckOnly:   false,
			UseLinkMode: false,
		})
		if err != nil {
			t.Errorf("returned error %+v", err)
		}

		stdout := stream.StdoutBuf.String()
		if stdout != StreamingMainStdout {
			t.Errorf("got stdout %q, want %q", stdout, StreamingMainStdout)
		}

		stderr := stream.StderrBuf.String()
		if stderr != StreamingMainStderr {
			t.Errorf("got stderr %q, want %q", stderr, StreamingMainStderr)
		}
	})

	t.Run("returns an error if the command succeeds but the io.Writer fails", func(t *testing.T) {
		// Don't fail in the subprocess even when the stdout stream is closed.
		SetExecCommand(exectest.NewCommand(BlindlyWritingMain))
		defer ResetExecCommand()

		SetRsyncExecCommand(exectest.NewCommand(Success))
		defer ResetRsyncExecCommand()

		expectedErr := errors.New("write failed!")
		err := UpgradeMaster(UpgradeMasterArgs{
			Source:      source,
			Target:      target,
			StateDir:    tempDir,
			Stream:      failingStreams{expectedErr},
			CheckOnly:   false,
			UseLinkMode: false,
		})
		if !xerrors.Is(err, expectedErr) {
			t.Errorf("returned error %+v, want %+v", err, expectedErr)
		}
	})

	t.Run("rsync during upgrade master errors out", func(t *testing.T) {
		SetExecCommand(exectest.NewCommand(StreamingMain))
		defer ResetExecCommand()

		SetRsyncExecCommand(exectest.NewCommand(Failure))
		defer ResetRsyncExecCommand()

		stream := new(step.BufferedStreams)

		err := UpgradeMaster(UpgradeMasterArgs{
			Source:      source,
			Target:      target,
			StateDir:    tempDir,
			Stream:      stream,
			CheckOnly:   false,
			UseLinkMode: false,
		})
		if err == nil {
			t.Errorf("expected error, returned nil")
		}

	})
}

func TestRsyncMasterDir(t *testing.T) {
	t.Run("rsync streams stdout and stderr to the client", func(t *testing.T) {
		SetRsyncExecCommand(exectest.NewCommand(StreamingMain))
		defer ResetRsyncExecCommand()

		stream := new(step.BufferedStreams)
		err := RsyncMasterDataDir(stream, "", "")

		if err != nil {
			t.Errorf("returned: %+v", err)
		}

		stdout := stream.StdoutBuf.String()
		if stdout != StreamingMainStdout {
			t.Errorf("got stdout %q, want %q", stdout, StreamingMainStdout)
		}

		stderr := stream.StderrBuf.String()
		if stderr != StreamingMainStderr {
			t.Errorf("got stderr %q, want %q", stderr, StreamingMainStderr)
		}
	})

}

// failingStreams is an implementation of OutStreams for which every call to a
// stream's Write() method will fail with the given error.
type failingStreams struct {
	err error
}

func (f failingStreams) Stdout() io.Writer {
	return &testutils.FailingWriter{f.err}
}

func (f failingStreams) Stderr() io.Writer {
	return &testutils.FailingWriter{f.err}
}
