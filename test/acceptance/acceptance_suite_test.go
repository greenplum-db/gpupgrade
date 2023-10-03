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

	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/greenplum/connection"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
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

func generate(t *testing.T, outputDir string) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "generate",
		"--non-interactive",
		"--gphome", GPHOME_SOURCE,
		"--port", PGPORT,
		"--seed-dir", filepath.Join(MustGetRepoRoot(t), "data-migration-scripts"),
		"--output-dir", filepath.Join(outputDir, "generated-scripts"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %#v stderr %s", err, output)
	}

	return strings.TrimSpace(string(output))
}

func apply(t *testing.T, gphome string, port string, phase idl.Step, inputDir string) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "apply",
		"--non-interactive",
		"--gphome", gphome,
		"--port", port,
		"--phase", phase.String(),
		"--input-dir", filepath.Join(inputDir, "generated-scripts"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %#v stderr %s", err, output)
	}

	return strings.TrimSpace(string(output))
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
		t.Fatalf("unexpected err: %#v stderr %s", err, output)
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
		t.Fatalf("unexpected err: %#v stderr %s", err, output)
	}

	return strings.TrimSpace(string(output))
}

func execute(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "execute",
		"--non-interactive", "--verbose")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %#v stderr %s", err, output)
	}

	return strings.TrimSpace(string(output))
}

func finalize(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "finalize",
		"--non-interactive", "--verbose")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %#v stderr %s", err, output)
	}

	return strings.TrimSpace(string(output))
}

func revert(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "revert",
		"--non-interactive", "--verbose")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %#v stderr %s", err, output)
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

func GetTargetCluster(t *testing.T) greenplum.Cluster {
	t.Helper()
	return getCluster(t, GPHOME_TARGET, testutils.MustConvertStringToInt(t, PGPORT), idl.ClusterDestination_target)
}

// GetTempTargetCluster creates a target cluster from the source cluster. It is
// used in a defer clause when a target cluster is needed for cleanup before
// the upgrade can be run to create the actual target cluster.
func GetTempTargetCluster(t *testing.T) greenplum.Cluster {
	t.Helper()

	source := GetSourceCluster(t)

	targetVersion, err := greenplum.Version(GPHOME_TARGET)
	if err != nil {
		t.Fatal(err)
	}

	tmpTarget := &source
	tmpTarget.Destination = idl.ClusterDestination_target
	tmpTarget.GPHome = GPHOME_TARGET
	tmpTarget.Version = targetVersion

	return *tmpTarget
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

// backupDemoCluster is used with restoreDemoCluster to restore a cluster after
// finalize.
func backupDemoCluster(t *testing.T, backupDir string, source greenplum.Cluster) {
	src := filepath.Dir(filepath.Dir(source.CoordinatorDataDir())) + string(os.PathSeparator)
	dest := backupDir + string(os.PathSeparator)

	testutils.MustCreateDir(t, dest)

	options := []rsync.Option{
		rsync.WithSources(src),
		rsync.WithDestination(dest),
		rsync.WithOptions(rsync.Options...),
		rsync.WithStream(step.DevNullStream),
	}
	err := rsync.Rsync(options...)
	if err != nil {
		t.Fatal(err)
	}
}

// restoreDemoCluster restores the cluster after finalize has been run.
func restoreDemoCluster(t *testing.T, backupDir string, source greenplum.Cluster, target greenplum.Cluster) {
	// Depending on where we failed we need to stop either the source or target cluster.
	err := source.Stop(step.DevNullStream)
	if err != nil {
		err = target.Stop(step.DevNullStream)
		if err != nil {
			t.Fatal(err)
		}
	}

	src := backupDir + string(os.PathSeparator)
	dest := filepath.Dir(filepath.Dir(source.CoordinatorDataDir())) + string(os.PathSeparator)

	options := []rsync.Option{
		rsync.WithSources(src),
		rsync.WithDestination(dest),
		rsync.WithOptions("--archive", "-I", "--delete"),
		rsync.WithStream(step.DevNullStream),
	}
	err = rsync.Rsync(options...)
	if err != nil {
		t.Fatal(err)
	}

	err = source.Start(step.DevNullStream)
	if err != nil {
		t.Fatal(err)
	}
}

func isolation2_regress(t *testing.T, sourceVersion semver.Version, gphome string, port string, inputDir string, outputDir string, schedule idl.Schedule) string {
	var cmdArgs []string
	if schedule != idl.Schedule_non_upgradeable_schedule && strings.Contains(schedule.String(), "target") {
		cmdArgs = append(cmdArgs, "--use-existing")
	}

	env := []string{"PGOPTIONS=-c optimizer=off"}
	var binDir string
	switch sourceVersion.Major {
	case 5:
		binDir = "--psqldir"
		// Set PYTHONPATH directly since it is needed when running the
		// pg_upgrade tests locally. Normally one would source
		// greenplum_path.sh, but that causes the following issues:
		// https://web.archive.org/web/20220506055918/https://groups.google.com/a/greenplum.org/g/gpdb-dev/c/JN-YwjCCReY/m/0L9wBOvlAQAJ
		env = append(env, "PYTHONPATH="+filepath.Join(GPHOME_SOURCE, "lib/python"))
	case 6:
		binDir = "--psqldir"
	default:
		binDir = "--bindir"
	}

	tests := "--schedule=" + filepath.Join(inputDir, schedule.String())
	focus := os.Getenv("FOCUS_TESTS")
	if focus != "" {
		tests = focus
	}

	cmdArgs = append(cmdArgs,
		"--init-file", "init_file_isolation2",
		"--inputdir", inputDir,
		"--outputdir", outputDir,
		binDir, filepath.Join(gphome, "bin"),
		"--port", port,
		tests,
	)

	cmd := exec.Command("./pg_isolation2_regress", cmdArgs...)
	cmd.Dir = testutils.MustGetEnv("ISOLATION2_PATH")
	cmd.Env = append(os.Environ(), env...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %#v stderr %s", err, output)
	}

	return strings.TrimSpace(string(output))
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
