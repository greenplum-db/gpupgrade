// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/greenplum/connection"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

var GPHOME_SOURCE string
var GPHOME_TARGET string
var PGPORT string

const TARGET_PGPORT = "6020"

func init() {
	gpHomeSource := os.Getenv("GPHOME_SOURCE")
	if gpHomeSource == "" {
		err := os.Setenv("GPHOME_SOURCE", testutils.MustGetEnv("GPHOME"))
		if err != nil {
			log.Fatalf("setting $GPHOME_SOURCE: %v", err)
		}
	}

	gpHomeTarget := os.Getenv("GPHOME_TARGET")
	if gpHomeTarget == "" {
		err := os.Setenv("GPHOME_TARGET", testutils.MustGetEnv("GPHOME"))
		if err != nil {
			log.Fatalf("setting $GPHOME_TARGET: %v", err)
		}
	}

	GPHOME_SOURCE = testutils.MustGetEnv("GPHOME_SOURCE")
	GPHOME_TARGET = testutils.MustGetEnv("GPHOME_TARGET")
	PGPORT = testutils.MustGetEnv("PGPORT")
}
func MustGetRepoRoot(t *testing.T) string {
	t.Helper()

	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}

	return filepath.Dir(filepath.Dir(currentDir))
}

func initialize_stopBeforeClusterCreation(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "initialize",
		"--non-interactive", "--verbose",
		"--source-gphome", GPHOME_SOURCE,
		"--target-gphome", GPHOME_TARGET,
		"--source-master-port", PGPORT,
		"--temp-port-range", TARGET_PGPORT+"-6040",
		"--stop-before-cluster-creation",
		"--disk-free-ratio", "0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %#v stderr %q", err, output)
	}

	return strings.TrimSpace(string(output))
}

func initialize(t *testing.T, mode idl.Mode) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "initialize",
		"--non-interactive", "--verbose",
		"--mode", mode.String(),
		"--source-gphome", GPHOME_SOURCE,
		"--target-gphome", GPHOME_TARGET,
		"--source-master-port", PGPORT,
		"--temp-port-range", TARGET_PGPORT+"-6040",
		"--disk-free-ratio", "0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %#v stderr %q", err, output)
	}

	return strings.TrimSpace(string(output))
}

func execute(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "execute",
		"--non-interactive", "--verbose")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %#v stderr %q", err, output)
	}

	return strings.TrimSpace(string(output))
}

func revert(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "revert",
		"--non-interactive", "--verbose")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %#v stderr %q", err, output)
	}

	return strings.TrimSpace(string(output))
}

func killServices(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "kill-services")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %v stderr: %q", err, output)
	}

	return strings.TrimSpace(string(output))
}

func restartServices(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "restart-services")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %v stderr: %q", err, output)
	}

	return strings.TrimSpace(string(output))
}

func GetSourceCluster(t *testing.T) greenplum.Cluster {
	t.Helper()
	return getCluster(t, GPHOME_SOURCE, testutils.MustConvertStringToInt(t, PGPORT), idl.ClusterDestination_source)
}

func GetIntermediateCluster(t *testing.T) greenplum.Cluster {
	t.Helper()
	return getCluster(t, GPHOME_TARGET, testutils.MustConvertStringToInt(t, TARGET_PGPORT), idl.ClusterDestination_intermediate)
}

func getCluster(t *testing.T, gphome string, port int, destination idl.ClusterDestination) greenplum.Cluster {
	t.Helper()

	db, err := connection.Bootstrap(destination, gphome, port)
	if err != nil {
		t.Fatalf("bootstraping db connection to %q %q %q: %v", destination, gphome, port, err)
	}
	defer func() {
		if cErr := db.Close(); cErr != nil {
			err = errorlist.Append(err, cErr)
		}
	}()

	cluster, err := greenplum.ClusterFromDB(db, gphome, destination)
	if err != nil {
		t.Fatalf("retrieve %s cluster configuration: %v", destination, err)
	}

	return cluster
}

func jq(t *testing.T, file string, args ...string) string {
	t.Helper()

	cmd := exec.Command("jq", append(args, "--raw-output", file)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %v stderr: %q", err, output)
	}

	return strings.TrimSpace(string(output))
}

func MustGetPgUpgradeLog(t *testing.T, contentID int32) string {
	t.Helper()

	dir, err := utils.GetPgUpgradeDir(greenplum.PrimaryRole, contentID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	return filepath.Join(dir, "pg_upgrade_internal.log")
}

func GetStatUtility() string {
	utility := "stat"
	if runtime.GOOS == "darwin" {
		utility = "gstat"
	}

	return utility
}

func MustGetLogArchiveDir(t *testing.T, upgradeID string) string {
	t.Helper()

	logDir, err := utils.GetLogDir()
	if err != nil {
		t.Fatalf("get log dir: %v", err)
	}

	return hub.GetLogArchiveDir(logDir, upgradeID, time.Now())
}
