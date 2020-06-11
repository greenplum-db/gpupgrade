// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package rsync_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

func TestRsync(t *testing.T) {
	testhelper.SetupTestLogger()

	if _, err := exec.LookPath("rsync"); err != nil {
		t.Fatalf("tests require rsync (%v)", err)
	}

	// TODO: add a test for using a remote host once no-install tests allow ssh to localhost

	t.Run("when source ends with os.PathSeparator, copies file to top level of destination", func(t *testing.T) {
		source := testutils.GetTempDir(t, "source")
		defer testutils.MustRemoveAll(t, source)

		destination := testutils.GetTempDir(t, "destination")
		defer testutils.MustRemoveAll(t, destination)

		filename := "file.txt"
		expected := []byte("hi")
		testutils.MustWriteToFile(t, filepath.Join(source, filename), expected)

		opts := []rsync.Option{
			rsync.WithSources(source + string(os.PathSeparator)),
			rsync.WithDestination(destination),
			rsync.WithOptions("--archive", "--delete"),
		}
		if err := rsync.Rsync(opts...); err != nil {
			t.Errorf("Rsync() returned error %+v", err)
		}

		actual := testutils.MustReadFile(t, filepath.Join(destination, "/", filename))
		if !bytes.Equal(actual, expected) {
			t.Errorf("got %v want %v", actual, expected)
		}
	})

	t.Run("when source does not end with os.PathSeparator, copies file to subdirectory of destination", func(t *testing.T) {
		source := testutils.GetTempDir(t, "source")
		defer testutils.MustRemoveAll(t, source)

		destination := testutils.GetTempDir(t, "destination")
		defer testutils.MustRemoveAll(t, destination)

		filename := "file.txt"
		expected := []byte("hi")
		testutils.MustWriteToFile(t, filepath.Join(source, filename), expected)

		opts := []rsync.Option{
			rsync.WithSources(source),
			rsync.WithDestination(destination),
			rsync.WithOptions("--archive", "--delete"),
		}
		if err := rsync.Rsync(opts...); err != nil {
			t.Errorf("Rsync() returned error %+v", err)
		}

		path := filepath.Join(destination, string(os.PathSeparator), filepath.Base(source), filename)
		actual := testutils.MustReadFile(t, path)
		if !bytes.Equal(actual, expected) {
			t.Errorf("got %v want %v", actual, expected)
		}
	})

	t.Run("copies multiple source directories to the destination directory", func(t *testing.T) {
		source := testutils.GetTempDir(t, "source")
		defer testutils.MustRemoveAll(t, source)

		source2 := testutils.GetTempDir(t, "source2")
		defer testutils.MustRemoveAll(t, source2)

		destination := testutils.GetTempDir(t, "destination")
		defer testutils.MustRemoveAll(t, destination)

		filename := "file.txt"
		expected := []byte("hi")
		testutils.MustWriteToFile(t, filepath.Join(source, filename), expected)

		filename2 := "file2.txt"
		expected2 := []byte("hi_2")
		testutils.MustWriteToFile(t, filepath.Join(source2, filename2), expected2)

		opts := []rsync.Option{
			rsync.WithSources(source, source2),
			rsync.WithDestination(destination),
			rsync.WithOptions("--archive", "--delete"),
		}
		if err := rsync.Rsync(opts...); err != nil {
			t.Errorf("Rsync() returned error %+v", err)
		}

		path := filepath.Join(destination, string(os.PathSeparator), filepath.Base(source), filename)
		actual := testutils.MustReadFile(t, path)
		if !bytes.Equal(actual, expected) {
			t.Errorf("got %v want %v", actual, expected)
		}

		path = filepath.Join(destination, string(os.PathSeparator), filepath.Base(source2), filename2)
		actual = testutils.MustReadFile(t, path)
		if !bytes.Equal(actual, expected2) {
			t.Errorf("got %v want %v", actual, expected2)
		}
	})

	t.Run("rsync writes to the passed in stream", func(t *testing.T) {
		source := testutils.GetTempDir(t, "source")
		defer testutils.MustRemoveAll(t, source)

		destination := testutils.GetTempDir(t, "destination")
		defer testutils.MustRemoveAll(t, destination)

		filename := "file.txt"
		expected := []byte("hi")
		testutils.MustWriteToFile(t, filepath.Join(source, filename), expected)

		streams := &step.BufferedStreams{}
		opts := []rsync.Option{
			rsync.WithSources(source + string(os.PathSeparator)),
			rsync.WithDestination(destination),
			rsync.WithOptions([]string{"--archive", "--verbose"}...),
			rsync.WithStream(streams),
		}
		if err := rsync.Rsync(opts...); err != nil {
			t.Errorf("Rsync() returned error %+v", err)
		}

		actual := streams.StdoutBuf.String()
		if !strings.Contains(actual, filename) {
			t.Errorf("expected stdout to contain filename %q but has %q", filename, actual)
		}

		path := filepath.Join(destination, string(os.PathSeparator), filename)
		contents := testutils.MustReadFile(t, path)
		if !bytes.Equal(contents, expected) {
			t.Errorf("got %v want %v", contents, expected)
		}
	})

	t.Run("when --delete is specified, it removes existing files", func(t *testing.T) {
		source := testutils.GetTempDir(t, "source")
		defer testutils.MustRemoveAll(t, source)

		destination := testutils.GetTempDir(t, "destination")
		defer testutils.MustRemoveAll(t, destination)

		filename := "filename.txt"
		testutils.MustWriteToFile(t, filepath.Join(destination, filename), []byte(""))

		opts := []rsync.Option{
			rsync.WithSources(source + string(os.PathSeparator)),
			rsync.WithDestination(destination),
			rsync.WithOptions("--archive", "--delete"),
		}
		if err := rsync.Rsync(opts...); err != nil {
			t.Errorf("Rsync() returned error %+v", err)
		}

		if pathExists(filepath.Join(destination, filename)) {
			t.Errorf("destination directory file %q should not exist, but it does", filename)
		}
	})

	t.Run("does not copy files in the exclusion list from the source directory", func(t *testing.T) {
		source := testutils.GetTempDir(t, "source")
		defer testutils.MustRemoveAll(t, source)

		destination := testutils.GetTempDir(t, "destination")
		defer testutils.MustRemoveAll(t, destination)

		filename := "filename.txt"
		testutils.MustWriteToFile(t, filepath.Join(source, filename), []byte(""))

		opts := []rsync.Option{
			rsync.WithSources(source + string(os.PathSeparator)),
			rsync.WithDestination(destination),
			rsync.WithOptions("--archive", "--delete"),
			rsync.WithExcludedFiles(filename),
		}
		if err := rsync.Rsync(opts...); err != nil {
			t.Errorf("Rsync() returned error %+v", err)
		}

		if pathExists(filepath.Join(destination, filename)) {
			t.Errorf("destination directory file %q should not exist, but it does", filename)
		}
	})

	t.Run("preserves files in the exclusion list in the destination directory", func(t *testing.T) {
		source := testutils.GetTempDir(t, "source")
		defer testutils.MustRemoveAll(t, source)

		destination := testutils.GetTempDir(t, "destination")
		defer testutils.MustRemoveAll(t, destination)

		filename1 := "filename1.txt"
		filename2 := "filename2.txt"
		filename3 := "filename3.txt"
		testutils.MustWriteToFile(t, filepath.Join(source, filename1), []byte(""))
		testutils.MustWriteToFile(t, filepath.Join(destination, filename2), []byte(""))
		testutils.MustWriteToFile(t, filepath.Join(destination, filename3), []byte(""))

		opts := []rsync.Option{
			rsync.WithSources(source + string(os.PathSeparator)),
			rsync.WithDestination(destination),
			rsync.WithOptions("--archive", "--delete"),
			rsync.WithExcludedFiles(filename2, filename3),
		}
		if err := rsync.Rsync(opts...); err != nil {
			t.Errorf("Rsync() returned error %+v", err)
		}

		if !pathExists(filepath.Join(destination, filename1)) {
			t.Errorf("file %q does not exist", filename1)
		}

		if !pathExists(filepath.Join(destination, filename2)) {
			t.Errorf("file %q does not exist", filename2)
		}

		if !pathExists(filepath.Join(destination, filename3)) {
			t.Errorf("file %q does not exist", filename3)
		}

	})

	t.Run("when an input stream is provided, it returns an RsyncError that wraps an ExitError", func(t *testing.T) {
		source := testutils.GetTempDir(t, "source")
		defer testutils.MustRemoveAll(t, source)

		destination := testutils.GetTempDir(t, "destination")
		defer testutils.MustRemoveAll(t, destination)

		testutils.MustWriteToFile(t, filepath.Join(source, "filename.txt"), []byte(""))

		stream := &step.BufferedStreams{}
		opts := []rsync.Option{
			rsync.WithSources(source + string(os.PathSeparator)),
			rsync.WithDestination(destination),
			rsync.WithOptions("--BOGUS"),
			rsync.WithStream(stream),
		}
		err := rsync.Rsync(opts...)
		if err == nil {
			t.Errorf("expected error, got nil")
		}

		var rsyncError rsync.RsyncError
		if !xerrors.As(err, &rsyncError) {
			t.Errorf("got error %#v, wanted type %T", err, rsyncError)
		}
		expected := "exit status 1"
		if rsyncError.Error() != expected {
			t.Errorf("got %s, expected %s", rsyncError.Error(), expected)
		}

		var exitError *exec.ExitError
		if !xerrors.As(err, &exitError) {
			t.Errorf("expected err to wrap ExitError")
		}
		if exitError.Error() != expected {
			t.Errorf("got %s, expected %s", err.Error(), expected)
		}

		expected = "rsync: --BOGUS: unknown option"
		if !strings.Contains(stream.StderrBuf.String(), expected) {
			t.Errorf("got %v, expected substring %s", stream.StderrBuf.String(), expected)
		}

	})

	t.Run("when no input stream is provided, it returns an RsyncError that wraps an ExitError", func(t *testing.T) {
		source := testutils.GetTempDir(t, "source")
		defer testutils.MustRemoveAll(t, source)

		destination := testutils.GetTempDir(t, "destination")
		defer testutils.MustRemoveAll(t, destination)

		testutils.MustWriteToFile(t, filepath.Join(source, "filename.txt"), []byte(""))

		opts := []rsync.Option{
			rsync.WithSources(source + string(os.PathSeparator)),
			rsync.WithDestination(destination),
			rsync.WithOptions("--BOGUS"),
		}
		err := rsync.Rsync(opts...)
		if err == nil {
			t.Errorf("expected error, got nil")
		}

		var rsyncError rsync.RsyncError
		if !xerrors.As(err, &rsyncError) {
			t.Errorf("got error %#v, wanted type %T", err, rsyncError)
		}
		expected := "rsync: --BOGUS: unknown option"
		if !strings.Contains(rsyncError.Error(), expected) {
			t.Errorf("got %s, expected %s", rsyncError.Error(), expected)
		}

		var exitError *exec.ExitError
		if !xerrors.As(err, &exitError) {
			t.Errorf("expected err to wrap ExitError")
		}
		if !strings.Contains(rsyncError.Error(), expected) {
			t.Errorf("got %v, expected substring %s", err.Error(), expected)
		}
	})
}

func pathExists(path string) bool {
	_, err := utils.System.Stat(path)
	return err == nil
}
