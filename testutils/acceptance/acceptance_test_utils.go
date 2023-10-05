// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package acceptance

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
	"github.com/greenplum-db/gpupgrade/upgrade"
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

	_, err := exec.LookPath("git")
	if err == nil {
		output, err := exec.Command("git", "rev-parse", "--show-toplevel").CombinedOutput()
		if err != nil {
			t.Fatalf("failed to get repository root: %v", err)
		}
		return strings.TrimSpace(string(output))
	} else {
		currentDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("failed to get current directory: %v", err)
		}
		return filepath.Dir(filepath.Dir(currentDir))
	}
}

func Generate(t *testing.T, outputDir string) string {
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

func Apply(t *testing.T, gphome string, port string, phase idl.Step, inputDir string) string {
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

func Initialize_stopBeforeClusterCreation(t *testing.T) string {
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

func Initialize(t *testing.T, mode idl.Mode) string {
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

func Execute(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "execute",
		"--non-interactive", "--verbose")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %#v stderr %s", err, output)
	}

	return strings.TrimSpace(string(output))
}

func Finalize(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "finalize",
		"--non-interactive", "--verbose")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %#v stderr %s", err, output)
	}

	return strings.TrimSpace(string(output))
}

func Revert(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "revert",
		"--non-interactive", "--verbose")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %#v stderr %s", err, output)
	}

	return strings.TrimSpace(string(output))
}

// RevertIgnoreFailures ignores failures since revert is part of the actual test
// calling revert a second time within a defer will fail. We call revert with a
// defer to clean up if the test fails part way through.
func RevertIgnoreFailures(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "revert",
		"--non-interactive", "--verbose")
	output, _ := cmd.CombinedOutput()

	return strings.TrimSpace(string(output))
}

func KillServices(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "kill-services")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %v stderr: %q", err, output)
	}

	return strings.TrimSpace(string(output))
}

func RestartServices(t *testing.T) string {
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
func BackupDemoCluster(t *testing.T, backupDir string, source greenplum.Cluster) {
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
func RestoreDemoCluster(t *testing.T, backupDir string, source greenplum.Cluster, target greenplum.Cluster) {
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

func Isolation2_regress(t *testing.T, sourceVersion semver.Version, gphome string, port string, inputDir string, outputDir string, schedule idl.Schedule) string {
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

func Jq(t *testing.T, file string, args ...string) string {
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

func CreateMarkerFilesOnMirrors(t *testing.T, mirrors greenplum.ContentToSegConfig) {
	t.Helper()

	for _, seg := range mirrors {
		testutils.MustWriteToRemoteFile(t, seg.Hostname, filepath.Join(seg.DataDir, "source-cluster.marker"), "")
	}
}

func RemoveMarkerFilesOnMirrors(t *testing.T, mirrors greenplum.ContentToSegConfig) {
	t.Helper()

	for _, seg := range mirrors {
		testutils.MustRemoveAllRemotely(t, seg.Hostname, filepath.Join(seg.DataDir, "source-cluster.marker"))
	}
}

func VerifyMarkerFilesOnPrimaries(t *testing.T, primaries greenplum.ContentToSegConfig, mode idl.Mode) {
	t.Helper()

	for _, seg := range primaries {
		if mode == idl.Mode_link {
			// in link mode revert uses rsync which copies over and retains the marker file
			testutils.RemotePathMustExist(t, seg.Hostname, filepath.Join(seg.DataDir, "source-cluster.marker"))
		}

		if mode == idl.Mode_copy {
			// in copy mode revert uses gprecoverseg which removes the marker file
			testutils.RemotePathMustNotExist(t, seg.Hostname, filepath.Join(seg.DataDir, "source-cluster.marker"))
		}

		testutils.MustRemoveAllRemotely(t, seg.Hostname, filepath.Join(seg.DataDir, "source-cluster.marker"))
	}
}

func CreateMarkerFilesOnAllSegments(t *testing.T, cluster greenplum.Cluster) {
	t.Helper()

	for _, seg := range cluster.Primaries {
		testutils.MustWriteToRemoteFile(t, seg.Hostname, filepath.Join(seg.DataDir, "source-cluster.marker"), "")
	}

	for _, seg := range cluster.Mirrors {
		testutils.MustWriteToRemoteFile(t, seg.Hostname, filepath.Join(seg.DataDir, "source-cluster.marker"), "")
	}
}

func RemoveMarkerFilesOnAllSegments(t *testing.T, cluster greenplum.Cluster) {
	t.Helper()

	for _, seg := range cluster.Primaries {
		testutils.MustRemoveAllRemotely(t, seg.Hostname, filepath.Join(seg.DataDir, "source-cluster.marker"))
	}

	for _, seg := range cluster.Mirrors {
		testutils.MustRemoveAllRemotely(t, seg.Hostname, filepath.Join(seg.DataDir, "source-cluster.marker"))
	}
}

func VerifyMarkerFilesOnAllSegments(t *testing.T, intermediate *greenplum.Cluster, target *greenplum.Cluster) {
	t.Helper()

	// Verify the source cluster has the marker files. Since the source cluster
	// got archived after finalize we take the intermediate cluster data
	// directories appended with the .old suffix as the archived source cluster
	// directories.
	for _, seg := range intermediate.Primaries {
		testutils.RemotePathMustExist(t, seg.Hostname, filepath.Join(seg.DataDir+upgrade.OldSuffix, "source-cluster.marker"))
		testutils.MustRemoveAllRemotely(t, seg.Hostname, filepath.Join(seg.DataDir+upgrade.OldSuffix, "source-cluster.marker"))
	}

	for _, seg := range intermediate.Mirrors {
		testutils.RemotePathMustExist(t, seg.Hostname, filepath.Join(seg.DataDir+upgrade.OldSuffix, "source-cluster.marker"))
		testutils.MustRemoveAllRemotely(t, seg.Hostname, filepath.Join(seg.DataDir+upgrade.OldSuffix, "source-cluster.marker"))
	}

	// Verify the target cluster does not have the marker files.
	for _, seg := range target.Primaries {
		testutils.RemotePathMustNotExist(t, seg.Hostname, filepath.Join(seg.DataDir, "source-cluster.marker"))
		testutils.MustRemoveAllRemotely(t, seg.Hostname, filepath.Join(seg.DataDir, "source-cluster.marker"))
	}

	for _, seg := range target.Mirrors {
		testutils.RemotePathMustNotExist(t, seg.Hostname, filepath.Join(seg.DataDir, "source-cluster.marker"))
		testutils.MustRemoveAllRemotely(t, seg.Hostname, filepath.Join(seg.DataDir, "source-cluster.marker"))
	}
}
