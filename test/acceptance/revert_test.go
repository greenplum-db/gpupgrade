// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/cli/commands"
	"github.com/greenplum-db/gpupgrade/config"
	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/substeps"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestRevert(t *testing.T) {
	stateDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, stateDir)

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

	t.Run("reverting after initialize succeeds", func(t *testing.T) {
		source := GetSourceCluster(t)

		initialize(t, idl.Mode_copy)
		defer revertIgnoreFailures(t) // cleanup in case we fail part way through

		conf, err := config.Read()
		if err != nil {
			t.Fatal(err)
		}

		output := revert(t)

		logArchiveDir := MustGetLogArchiveDir(t, conf.UpgradeID)
		verifyRevert(t, source, conf.Intermediate, output, logArchiveDir)
	})

	t.Run("reverting after initialize exits early", func(t *testing.T) {
		cmd := exec.Command("gpupgrade", "initialize",
			"--verbose",
			"--mode", idl.Mode_copy.String(),
			"--source-gphome", GPHOME_SOURCE,
			"--target-gphome", GPHOME_TARGET,
			"--source-master-port", PGPORT,
			"--temp-port-range", TARGET_PGPORT+"-6040",
			"--disk-free-ratio", "0",
			"--seed-dir", filepath.Join(MustGetRepoRoot(t), "data-migration-scripts"))
		cmd.Stdin = strings.NewReader("y\nq\n") // cause initialize to exit early
		output, err := cmd.CombinedOutput()
		defer revertIgnoreFailures(t) // cleanup in case we fail part way through
		if err != nil && strings.HasSuffix(err.Error(), step.Quit.Error()) {
			log.Fatalf("unexpected err: %#v stderr %q", err, output)
		}

		conf, err := config.Read()
		if err != nil {
			t.Fatal(err)
		}

		revert(t)

		// Since the logArchiveDir has a timestamp we need to do a partial check
		logArchiveDir := MustGetLogArchiveDir(t, conf.UpgradeID)
		logArchiveDir = logArchiveDir[:len(logArchiveDir)-5] + "*"

		testutils.RemotePathMustExist(t, conf.Intermediate.CoordinatorHostname(), logArchiveDir)

		for _, host := range conf.Intermediate.Hosts() {
			testutils.RemoteProcessMustNotBeRunning(t, host, "[g]pupgrade hub")
			testutils.RemoteProcessMustNotBeRunning(t, host, "[g]pupgrade agent")

			testutils.RemotePathMustNotExist(t, host, utils.GetStateDir())
		}
	})

	t.Run("reverting after execute in link mode succeeds", func(t *testing.T) {
		makeNoUpgradeFailure := UpgradeFailure{
			setup:         noUpgradeFailureToSetup,
			revert:        noUpgradeFailureToRevert,
			failedSubstep: idl.Substep_unknown_substep,
		}

		testRevertAfterExecute(t, idl.Mode_link, makeNoUpgradeFailure)
	})

	t.Run("reverting after execute in copy mode succeeds", func(t *testing.T) {
		makeNoUpgradeFailure := UpgradeFailure{
			setup:         noUpgradeFailureToSetup,
			revert:        noUpgradeFailureToRevert,
			failedSubstep: idl.Substep_unknown_substep,
		}

		testRevertAfterExecute(t, idl.Mode_copy, makeNoUpgradeFailure)
	})

	t.Run("reverting succeeds after copy-mode execute fails while upgrading coordinator", func(t *testing.T) {
		makeUpgradeCoordinatorFail := UpgradeFailure{
			setup:         setupCoordinatorUpgradeFailure,
			revert:        revertCoordinatorUpgradeFailure,
			failedSubstep: idl.Substep_upgrade_master,
		}

		testRevertAfterExecute(t, idl.Mode_copy, makeUpgradeCoordinatorFail)
	})

	t.Run("reverting succeeds after link-mode execute fails while upgrading coordinator", func(t *testing.T) {
		makeUpgradeCoordinatorFail := UpgradeFailure{
			setup:         setupCoordinatorUpgradeFailure,
			revert:        revertCoordinatorUpgradeFailure,
			failedSubstep: idl.Substep_upgrade_master,
		}

		testRevertAfterExecute(t, idl.Mode_link, makeUpgradeCoordinatorFail)
	})

	t.Run("reverting succeeds after copy-mode execute fails while upgrading primary segments", func(t *testing.T) {
		makeUpgradePrimariesFail := UpgradeFailure{
			setup:         setupPrimaryUpgradeFailure,
			revert:        revertPrimaryUpgradeFailure,
			failedSubstep: idl.Substep_upgrade_primaries,
		}

		testRevertAfterExecute(t, idl.Mode_copy, makeUpgradePrimariesFail)
	})

	t.Run("reverting succeeds after link-mode execute fails while upgrading primary segments", func(t *testing.T) {
		makeUpgradePrimariesFail := UpgradeFailure{
			setup:         setupPrimaryUpgradeFailure,
			revert:        revertPrimaryUpgradeFailure,
			failedSubstep: idl.Substep_upgrade_primaries,
		}

		testRevertAfterExecute(t, idl.Mode_link, makeUpgradePrimariesFail)
	})

	t.Run("can successfully run gpupgrade after a revert", func(t *testing.T) {
		initialize(t, idl.Mode_copy)
		execute(t)
		revert(t)

		initialize(t, idl.Mode_link)
		execute(t)
		revert(t)
	})
}

func testRevertAfterExecute(t *testing.T, mode idl.Mode, upgradeFailure UpgradeFailure) {
	source := GetSourceCluster(t)

	// setup upgrade failure
	path := upgradeFailure.setup(t, source)
	defer upgradeFailure.revert(t, source, path)

	// place marker files on source cluster mirrors to ensure primaries are correctly reverted using the mirrors
	createMarkerFilesOnMirrors(t, source.Mirrors)
	defer removeMarkerFilesOnMirrors(t, source.Mirrors)

	// add a table
	table := "public.should_be_reverted"
	testutils.MustExecuteSQL(t, source.Connection(greenplum.Database("postgres")), fmt.Sprintf(`CREATE TABLE %s (a int); INSERT INTO %s VALUES (1), (2), (3);`, table, table))
	defer testutils.MustExecuteSQL(t, source.Connection(greenplum.Database("postgres")), fmt.Sprintf(`DROP TABLE %s;`, table))

	tempDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, tempDir)

	// add tablespace
	testutils.MustAddTablespace(t, source, tempDir)
	defer testutils.MustDeleteTablespaces(t, source)

	// dump all databases before the upgrade
	err := source.RunGreenplumCmd(step.NewLogStdStreams(false), "pg_dumpall", "--schema-only", "-f", filepath.Join(tempDir, "before.sql"))
	if err != nil {
		t.Fatal(err)
	}

	// run initialize and execute
	initialize(t, mode)
	defer revertIgnoreFailures(t) // cleanup in case we fail part way through
	upgradeSucceeded := verifyExecute(t, upgradeFailure.failedSubstep)

	if upgradeSucceeded {
		intermediate := GetIntermediateCluster(t)

		// modify a table on the intermediate cluster to ensure it is properly reverted
		testutils.MustExecuteSQL(t, intermediate.Connection(greenplum.Database("postgres")), fmt.Sprintf(`TRUNCATE TABLE %s;`, table))

		// modify tablespace data on the intermediate cluster to ensure it is properly reverted
		testutils.MustTruncateTablespaces(t, intermediate)
	}

	conf, err := config.Read()
	if err != nil {
		t.Fatal(err)
	}

	// revert
	revertOutput := revert(t)

	logArchiveDir := MustGetLogArchiveDir(t, conf.UpgradeID)
	verifyRevert(t, source, conf.Intermediate, revertOutput, logArchiveDir)

	// verify that the mirror marker files were restored to the primaries after reverting
	verifyMarkerFilesOnPrimaries(t, source.Primaries, mode)

	// verify the table modifications were reverted
	rows := testutils.MustQueryRow(t, source.Connection(greenplum.Database("postgres")), fmt.Sprintf(`SELECT COUNT(*) FROM %s;`, table))
	expected := 3
	if rows != expected {
		t.Fatalf("got %d want %d rows", rows, expected)
	}

	// verify tablespace modifications were reverted
	testutils.VerifyTablespaceData(t, source)

	// dump all databases after the upgrade
	err = source.RunGreenplumCmd(step.NewLogStdStreams(false), "pg_dumpall", "--schema-only", "-f", filepath.Join(tempDir, "after.sql"))
	if err != nil {
		t.Fatal(err)
	}

	// compare the dumps
	cmd := exec.Command("diff", "-U3", "--speed-large-files", filepath.Join(tempDir, "before.sql"), filepath.Join(tempDir, "after.sql"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %v stderr: %q", err, output)
	}
}

func verifyRevert(t *testing.T, source greenplum.Cluster, intermediate *greenplum.Cluster, revertOutput string, logArchiveDir string) {
	t.Helper()

	// Since the logArchiveDir has a timestamp we need to do a partial check
	logArchiveDir = logArchiveDir[:len(logArchiveDir)-5]

	match := fmt.Sprintf(commands.RevertCompletedText,
		source.Version,
		filepath.Join(source.GPHome, "greenplum_path.sh"), source.CoordinatorDataDir(), source.CoordinatorPort(),
		logArchiveDir+`\d{5}`,
		idl.Step_revert,
		source.GPHome, source.CoordinatorPort(), filepath.Join(logArchiveDir+`\d{5}`, "data-migration-scripts"), idl.Step_revert)
	expected := regexp.MustCompile(match)
	if !expected.MatchString(revertOutput) {
		t.Fatalf("expected %q to contain %v", revertOutput, expected)
	}

	for _, host := range intermediate.Hosts() {
		testutils.RemoteProcessMustNotBeRunning(t, host, "[g]pupgrade hub")
		testutils.RemoteProcessMustNotBeRunning(t, host, "[g]pupgrade agent")

		testutils.RemotePathMustNotExist(t, host, utils.GetStateDir())

		testutils.RemotePathMustExist(t, host, logArchiveDir+"*")
	}

	for _, seg := range intermediate.Primaries {
		testutils.RemotePathMustNotExist(t, seg.Hostname, seg.DataDir)
	}

	testutils.VerifyClusterIsStopped(t, *intermediate)

	// verify configuration matches before and after reverting
	if !reflect.DeepEqual(source, GetSourceCluster(t)) {
		t.Errorf("expected source cluster to match before and after upgrading.")
		t.Errorf("got: %v", source)
		t.Errorf("want: %v", GetSourceCluster(t))
	}

	err := source.WaitForClusterToBeReady()
	if err != nil {
		t.Fatal(err)
	}
}

// revertIgnoreFailures ignores failures since revert is part of the actual test
// calling revert a second time within a defer will fail. We call revert with a
// defer to clean up if the test fails part way through.
func revertIgnoreFailures(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "revert",
		"--non-interactive", "--verbose")
	output, _ := cmd.CombinedOutput()

	return strings.TrimSpace(string(output))
}

func createMarkerFilesOnMirrors(t *testing.T, mirrors greenplum.ContentToSegConfig) {
	t.Helper()

	for _, seg := range mirrors {
		testutils.MustWriteToRemoteFile(t, seg.Hostname, filepath.Join(seg.DataDir, "source-cluster.marker"), "")
	}
}

func removeMarkerFilesOnMirrors(t *testing.T, mirrors greenplum.ContentToSegConfig) {
	t.Helper()

	for _, seg := range mirrors {
		testutils.MustRemoveAllRemotely(t, seg.Hostname, filepath.Join(seg.DataDir, "source-cluster.marker"))
	}
}

func verifyMarkerFilesOnPrimaries(t *testing.T, primaries greenplum.ContentToSegConfig, mode idl.Mode) {
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

func verifyExecute(t *testing.T, failedSubstep idl.Substep) bool {
	t.Helper()

	cmd := exec.Command("gpupgrade", "execute", "--non-interactive", "--verbose")
	output, err := cmd.CombinedOutput()
	executeOutput := strings.TrimSpace(string(output))
	expectExecuteToFail := failedSubstep != idl.Substep_unknown_substep
	if !expectExecuteToFail && err != nil {
		t.Fatalf("unexpected err: %#v stderr %q", err, output)
	}

	substepOutputText := substeps.SubstepDescriptions[failedSubstep].OutputText
	failedSubstepText := commanders.Format(substepOutputText, idl.Status_failed)
	if expectExecuteToFail && !strings.Contains(executeOutput, failedSubstepText) {
		t.Fatalf("expected execute to fail with %q got %q", failedSubstepText, executeOutput)
	}

	return !expectExecuteToFail
}

type UpgradeFailure struct {
	setup         func(t *testing.T, source greenplum.Cluster) string
	revert        func(t *testing.T, cluster greenplum.Cluster, path string)
	failedSubstep idl.Substep
}

func noUpgradeFailureToSetup(t *testing.T, source greenplum.Cluster) string {
	return ""
}

func noUpgradeFailureToRevert(t *testing.T, source greenplum.Cluster, path string) {

}

func setupCoordinatorUpgradeFailure(t *testing.T, source greenplum.Cluster) string {
	t.Helper()

	conn := source.Connection(greenplum.Database("postgres"))
	testutils.MustExecuteSQL(t, conn, `CREATE TABLE public.coordinator_failure (a int, b int); INSERT INTO public.coordinator_failure SELECT i, i FROM generate_series(1,10) i;`)
	relfile := testutils.MustQueryRow(t, conn, `SELECT relfilenode FROM pg_class WHERE relname='coordinator_failure';`)
	dbOid := testutils.MustQueryRow(t, conn, `SELECT oid FROM pg_database WHERE datname='postgres';`)

	path := filepath.Join(source.CoordinatorDataDir(), "base", strconv.Itoa(dbOid), strconv.Itoa(relfile))
	err := os.Rename(path, path+".bak")
	if err != nil {
		t.Fatal(err)
	}

	return path
}

func revertCoordinatorUpgradeFailure(t *testing.T, source greenplum.Cluster, path string) {
	t.Helper()

	err := os.Rename(path+".bak", path)
	if err != nil {
		t.Fatal(err)
	}

	testutils.MustExecuteSQL(t, source.Connection(greenplum.Database("postgres")), `DROP TABLE IF EXISTS public.coordinator_failure;`)
}

func setupPrimaryUpgradeFailure(t *testing.T, source greenplum.Cluster) string {
	t.Helper()

	conn := source.Connection(greenplum.Database("postgres"))
	testutils.MustExecuteSQL(t, conn, `CREATE TABLE public.primary_failure (a int, b int); INSERT INTO public.primary_failure SELECT i, i FROM generate_series(1,10) i;`)
	relfile := testutils.MustQueryRow(t, conn, `SELECT relfilenode FROM gp_dist_random('pg_class') WHERE relname='primary_failure' AND gp_segment_id=0;`)
	dbOid := testutils.MustQueryRow(t, conn, `SELECT oid FROM gp_dist_random('pg_database') WHERE datname='postgres' AND gp_segment_id=0;`)

	// NOTE: Before removing the relfile for primary_failure_tbl issue a checkpoint to flush the dirty buffers to disk.
	// Later we have a CREATE DATABASE statement which indirectly creates a checkpoint and if the dirty buffers exist at
	// that point the statement will fail.
	testutils.MustExecuteSQL(t, conn, `CHECKPOINT;`)

	path := filepath.Join(source.Primaries[0].DataDir, "base", strconv.Itoa(dbOid), strconv.Itoa(relfile))
	testutils.MustMoveRemoteFile(t, source.Primaries[0].Hostname, path, path+".bak")
	return path
}

func revertPrimaryUpgradeFailure(t *testing.T, source greenplum.Cluster, path string) {
	t.Helper()

	testutils.MustMoveRemoteFile(t, source.Primaries[0].Hostname, path+".bak", path)
	testutils.MustExecuteSQL(t, source.Connection(greenplum.Database("postgres")), `DROP TABLE IF EXISTS public.primary_failure;`)
}
