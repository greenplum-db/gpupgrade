// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package testutils

import (
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

// FailingWriter is an io.Writer for which all calls to Write() return an error.
type FailingWriter struct {
	Err error
}

func (f *FailingWriter) Write(_ []byte) (int, error) {
	return 0, f.Err
}

// TODO remove in favor of MustCreateCluster
func CreateMultinodeSampleCluster(baseDir string) *greenplum.Cluster {
	return &greenplum.Cluster{
		ContentIDs: []int{-1, 0, 1},
		Primaries: map[int]greenplum.SegConfig{
			-1: {ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: baseDir + "/seg-1", Role: "p"},
			0:  {ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: baseDir + "/seg1", Role: "p"},
			1:  {ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: baseDir + "/seg2", Role: "p"},
		},
	}
}

// TODO remove in favor of MustCreateCluster
func CreateMultinodeSampleClusterPair(baseDir string) (*greenplum.Cluster, *greenplum.Cluster) {
	gpdbVersion := dbconn.NewVersion("6.0.0")

	sourceCluster := CreateMultinodeSampleCluster(baseDir)
	sourceCluster.GPHome = "/usr/local/source"
	sourceCluster.Version = gpdbVersion

	targetCluster := CreateMultinodeSampleCluster(baseDir)
	targetCluster.GPHome = "/usr/local/target"
	targetCluster.Version = gpdbVersion

	return sourceCluster, targetCluster
}

func MustGetPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal("failed to listen on tcp:0")
	}
	defer func() {
		err = listener.Close()
		if err != nil {
			t.Fatal("failed to close listener")
		}
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	t.Logf("found available port %d", port)
	return port
}

func GetTempDir(t *testing.T, prefix string) string {
	t.Helper()

	dir, err := ioutil.TempDir("", prefix+"-")
	if err != nil {
		t.Fatalf("creating temporary directory: %+v", err)
	}

	return dir
}

func MustRemoveAll(t *testing.T, dir string) {
	t.Helper()

	err := os.RemoveAll(dir)
	if err != nil {
		t.Fatalf("removing temp dir %q: %+v", dir, err)
	}
}

// MustCreateDataDirs returns a temporary source and target data directory that
// looks like a postgres directory. The last argument returned is a cleanup
// function that can be used in a defer.
func MustCreateDataDirs(t *testing.T) (string, string, func(*testing.T)) {
	t.Helper()

	source := GetTempDir(t, "source")
	target := GetTempDir(t, "target")

	for _, dir := range []string{source, target} {
		for _, f := range upgrade.PostgresFiles {
			path := filepath.Join(dir, f)
			err := ioutil.WriteFile(path, []byte(""), 0600)
			if err != nil {
				t.Fatalf("failed creating postgres file %q: %+v", path, err)
			}
		}
	}

	return source, target, func(t *testing.T) {
		if err := os.RemoveAll(source); err != nil {
			t.Errorf("removing source directory: %v", err)
		}
		if err := os.RemoveAll(target); err != nil {
			t.Errorf("removing target directory: %v", err)
		}
	}
}

func MustReadFile(t *testing.T, path string) string {
	t.Helper()

	buf, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("error reading file %q: %v", path, err)
	}

	return string(buf)
}

func MustWriteToFile(t *testing.T, path string, contents string) {
	t.Helper()

	err := ioutil.WriteFile(path, []byte(contents), 0600)
	if err != nil {
		t.Fatalf("error writing file %q: %v", path, err)
	}
}

// VerifyRename ensures the source and archive data directories exist, and the
// target directory does not exist.
func VerifyRename(t *testing.T, source, target string) {
	t.Helper()

	if !upgrade.PathExists(source) {
		t.Errorf("expected source %q to exist", source)
	}

	archive := target + upgrade.OldSuffix
	if !upgrade.PathExists(archive) {
		t.Errorf("expected archive %q to exist", archive)
	}

	if upgrade.PathExists(target) {
		t.Errorf("expected target %q to not exist", target)
	}
}

func SetEnv(t *testing.T, envar, value string) func() {
	t.Helper()

	old, reset := os.LookupEnv(envar)

	err := os.Setenv(envar, value)
	if err != nil {
		t.Fatalf("setting %s environment variable to %s: %#v", envar, value, err)
	}

	return func() {
		if reset {
			err := os.Setenv(envar, old)
			if err != nil {
				t.Fatalf("resetting %s environment variable to %s: %#v", envar, old, err)
			}
		} else {
			err := os.Unsetenv(envar)
			if err != nil {
				t.Fatalf("unsetting %s environment variable: %#v", envar, err)
			}
		}
	}
}

// MustClearEnv makes sure envar is cleared, and returns a function to be used
// in a defer that resets the state to what it was prior to this function being called.
func MustClearEnv(t *testing.T, envar string) func() {
	t.Helper()

	old, reset := os.LookupEnv(envar)

	if reset {
		err := os.Unsetenv(envar)
		if err != nil {
			t.Fatalf("unsetting %s environment variable: %#v", envar, err)
		}
	}

	return func() {
		if reset {
			err := os.Setenv(envar, old)
			if err != nil {
				t.Fatalf("resetting %s environment variable to %s: %#v", envar, old, err)
			}
		} else {
			err := os.Unsetenv(envar)
			if err != nil {
				t.Fatalf("unsetting %s environment variable: %#v", envar, err)
			}
		}
	}
}

// MustMakeTablespaceDir returns a temporary tablespace directory, its parent
// dbID directory, and its grandparent tablespace location. The location should
// be removed for cleanup.
func MustMakeTablespaceDir(t *testing.T, tablespaceOid int) (string, string, string) {
	t.Helper()

	// ex: /filespace/demoDataDir0
	filespace := os.TempDir()

	// ex: /filespace/demoDataDir0/16386
	if tablespaceOid == 0 {
		tablespaceOid = 16386
	}
	location := filepath.Join(filespace, strconv.Itoa(tablespaceOid))
	err := os.MkdirAll(location, 0700)
	if err != nil {
		t.Fatalf("creating tablespace location directory: %v", err)
	}

	// ex: /filespace/demoDataDir0/16386/1
	dbID := filepath.Join(location, "1")
	err = os.MkdirAll(dbID, 0700)
	if err != nil {
		t.Fatalf("creating tablespace dbID directory: %v", err)
	}

	// ex: /filespace/demoDataDir0/16386/1/GPDB_6_301908232
	tablespace := filepath.Join(dbID, "GPDB_6_301908232")
	err = os.MkdirAll(tablespace, 0700)
	if err != nil {
		t.Fatalf("creating tablespace directory: %v", err)
	}

	return tablespace, dbID, location
}

// MustMake5XTablespaceDir returns the parent tablespace location and dbOID
// directory containing PG_VERSION and a relfile. The location should be removed
// for cleanup. An optional tablespaceOid may be specified to construct unique
// tablespace directories.
func MustMake5XTablespaceDir(t *testing.T, tablespaceOID int) (string, string) {
	t.Helper()

	// ex: /filespace/demoDataDir0
	filespace := os.TempDir()

	// ex: /filespace/demoDataDir0/16386
	if tablespaceOID == 0 {
		tablespaceOID = 16386
	}
	location := filepath.Join(filespace, strconv.Itoa(tablespaceOID))
	err := os.MkdirAll(location, 0700)
	if err != nil {
		t.Fatalf("creating 5x tablespace location directory: %v", err)
	}

	// ex: /filespace/demoDataDir0/16386/12094
	dbOID := filepath.Join(location, "12094")
	err = os.MkdirAll(dbOID, 0700)
	if err != nil {
		t.Fatalf("creating 5x tablespace dbOID directory: %v", err)
	}

	MustWriteToFile(t, filepath.Join(dbOID, upgrade.PGVersion), "")
	MustWriteToFile(t, filepath.Join(dbOID, "16384"), "")

	return dbOID, location
}

func MustGetExecutablePath(t *testing.T) string {
	t.Helper()

	path, err := os.Executable()
	if err != nil {
		t.Fatalf("failed getting test executable path: %#v", err)
	}

	return filepath.Dir(path)
}
