// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package testutils

import (
	"io/ioutil"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/onsi/gomega/gbytes"

	"github.com/greenplum-db/gpupgrade/greenplum"
)

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
	sourceCluster.BinDir = "/source/bindir"
	sourceCluster.Version = gpdbVersion

	targetCluster := CreateMultinodeSampleCluster(baseDir)
	targetCluster.BinDir = "/target/bindir"
	targetCluster.Version = gpdbVersion

	return sourceCluster, targetCluster
}

func GetOpenPort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()

	return l.Addr().(*net.TCPAddr).Port, nil
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
		t.Fatalf("removing temp dir %q: %#v", dir, err)
	}
}

func SetEnv(t *testing.T, envar, value string) func() {
	t.Helper()

	old := os.Getenv(envar)

	err := os.Setenv(envar, value)
	if err != nil {
		t.Fatalf("setting %s environment variable to %s", envar, value)
	}

	return func() {
		err := os.Setenv(envar, old)
		if err != nil {
			t.Fatalf("setting %s environment variable to %s", envar, old)
		}
	}
}

func VerifyLogContains(t *testing.T, testlog *gbytes.Buffer, expected string) {
	verifyLog(t, testlog, expected, true)
}

func VerifyLogDoesNotContain(t *testing.T, testlog *gbytes.Buffer, expected string) {
	verifyLog(t, testlog, expected, false)
}

func verifyLog(t *testing.T, testlog *gbytes.Buffer, expected string, shouldContain bool) {
	text := "to not contain"
	if shouldContain {
		text = "to contain"
	}

	contents := string(testlog.Contents())
	if shouldContain && !strings.Contains(contents, expected) {
		t.Errorf("\nexpected log: %q\n%s:   %q", contents, text, expected)
	}
}
