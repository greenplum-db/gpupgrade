// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"bufio"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/cli/commands"
	"github.com/greenplum-db/gpupgrade/config"
	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/acceptance"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestFinalize(t *testing.T) {
	stateDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, stateDir)

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

	t.Run("in copy mode gpupgrade finalize should swap the target data directories and ports with the source cluster", func(t *testing.T) {
		testFinalize(t, idl.Mode_copy, false)
	})

	t.Run("in link mode gpupgrade finalize should also delete mirror directories and honors --use-hba-hostnames", func(t *testing.T) {
		testFinalize(t, idl.Mode_link, true)
	})
}

func testFinalize(t *testing.T, mode idl.Mode, useHbaHostnames bool) {
	source := acceptance.GetSourceCluster(t)

	backupDir := testutils.GetTempDir(t, "backup")
	defer testutils.MustRemoveAll(t, backupDir)

	acceptance.BackupDemoCluster(t, backupDir, source)
	defer acceptance.RestoreDemoCluster(t, backupDir, source, acceptance.GetTempTargetCluster(t))

	createMarkerFilesOnAllSegments(t, source)
	defer removeMarkerFilesOnAllSegments(t, source)

	table := "public.should_be_reverted"
	testutils.MustExecuteSQL(t, source.Connection(greenplum.Database("postgres")), fmt.Sprintf(`CREATE TABLE %s (a int); INSERT INTO %s VALUES (1), (2), (3);`, table, table))
	defer testutils.MustExecuteSQL(t, source.Connection(greenplum.Database("postgres")), fmt.Sprintf(`DROP TABLE %s;`, table))

	tablespaceDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, tablespaceDir)

	testutils.MustAddTablespace(t, source, tablespaceDir)
	defer testutils.MustDeleteTablespaces(t, source, acceptance.GetTempTargetCluster(t))

	hbaHostnames := ""
	if useHbaHostnames {
		hbaHostnames = "--use-hba-hostnames"
	}

	cmd := exec.Command("gpupgrade", "initialize",
		"--non-interactive", "--verbose",
		"--mode", mode.String(),
		"--source-gphome", acceptance.GPHOME_SOURCE,
		"--target-gphome", acceptance.GPHOME_TARGET,
		"--source-master-port", acceptance.PGPORT,
		"--temp-port-range", acceptance.TARGET_PGPORT+"-6040",
		"--disk-free-ratio", "0",
		hbaHostnames)
	output, err := cmd.CombinedOutput()
	defer revertIgnoreFailures(t) // cleanup in case we fail part way through
	if err != nil {
		t.Fatalf("unexpected err: %#v stderr %s", err, output)
	}

	acceptance.Execute(t)

	conf, err := config.Read()
	if err != nil {
		t.Fatal(err)
	}

	finalizeOutput := acceptance.Finalize(t)

	verifyFinalize(t, source, conf, finalizeOutput, useHbaHostnames)

	verifyMarkerFilesOnAllSegments(t, conf.Intermediate, conf.Target)

	rows := testutils.MustQueryRow(t, conf.Target.Connection(greenplum.Database("postgres")), fmt.Sprintf(`SELECT COUNT(*) FROM %s;`, table))
	expectedRows := 3
	if rows != expectedRows {
		t.Fatalf("got %d want %d rows", rows, expectedRows)
	}

	testutils.VerifyTablespaceData(t, *conf.Target)

	// FIXME: When this script is run in link mode then the above call to remove tablespaces fails with:
	//   ERROR: failed to acquire resources on one or more segments (SQLSTATE 58M01)
	// Specifically, it errors when accessing objects in a database created in a tablespace. That is, accessing a table
	// from a database created in a tablespace when this test is run in link mode. For example, connecting to foodb and
	// doing \d+ errors with:
	//   ERROR:  failed to acquire resources on one or more segments
	//   DETAIL:  FATAL:  could not read relation mapping file "pg_tblspc/16389/GPDB_6_301908232/16487/pg_filenode.map": Success (relmapper.c:660)
	// Connecting to one of the segments errors with:
	//   PGOPTIONS="-c gp_session_role=utility" psql -p 25432 foodb
	//   psql: FATAL:  could not read relation mapping file "pg_tblspc/16389/GPDB_6_301908232/16487/pg_filenode.map": Success (relmapper.c:660)
	// Inspecting the tablespace location is extremely chaotic especially after upgrading!
	if mode != idl.Mode_link {
		path := filepath.Join(acceptance.MustGetRepoRoot(t), "test", "acceptance", "helpers", "finalize_checks.bash")
		script := fmt.Sprintf("source %s; validate_mirrors_and_standby %s %s %s", path, acceptance.GPHOME_TARGET, conf.Target.CoordinatorHostname(), acceptance.PGPORT)
		cmd = exec.Command("bash", "-c", script)
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("unexpected err: %#v stderr %s", err, output)
		}
	}
}

func createMarkerFilesOnAllSegments(t *testing.T, cluster greenplum.Cluster) {
	t.Helper()

	for _, seg := range cluster.Primaries {
		testutils.MustWriteToRemoteFile(t, seg.Hostname, filepath.Join(seg.DataDir, "source-cluster.marker"), "")
	}

	for _, seg := range cluster.Mirrors {
		testutils.MustWriteToRemoteFile(t, seg.Hostname, filepath.Join(seg.DataDir, "source-cluster.marker"), "")
	}
}

func removeMarkerFilesOnAllSegments(t *testing.T, cluster greenplum.Cluster) {
	t.Helper()

	for _, seg := range cluster.Primaries {
		testutils.MustRemoveAllRemotely(t, seg.Hostname, filepath.Join(seg.DataDir, "source-cluster.marker"))
	}

	for _, seg := range cluster.Mirrors {
		testutils.MustRemoveAllRemotely(t, seg.Hostname, filepath.Join(seg.DataDir, "source-cluster.marker"))
	}
}

func verifyMarkerFilesOnAllSegments(t *testing.T, intermediate *greenplum.Cluster, target *greenplum.Cluster) {
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

func verifyFinalize(t *testing.T, source greenplum.Cluster, conf *config.Config, finalizeOutput string, useHbaHostnames bool) {
	t.Helper()

	// Since the logArchiveDir has a timestamp we need to do a partial check
	logArchiveDir := acceptance.MustGetLogArchiveDir(t, conf.UpgradeID)
	logArchiveDir = logArchiveDir[:len(logArchiveDir)-5]

	match := fmt.Sprintf(commands.FinalizeCompletedText,
		conf.Target.Version,
		fmt.Sprintf("%s.<contentID>%s", conf.UpgradeID, upgrade.OldSuffix),
		conf.Intermediate.CoordinatorDataDir()+upgrade.OldSuffix,
		logArchiveDir+`\d{5}`,
		filepath.Join(conf.Target.GPHome, "greenplum_path.sh"),
		filepath.Join(filepath.Dir(conf.Target.GPHome), "greenplum-db"), conf.Target.GPHome,
		filepath.Join(conf.Target.GPHome, "greenplum_path.sh"),
		conf.Target.CoordinatorDataDir(),
		conf.Target.CoordinatorPort(),
		idl.Step_finalize,
		conf.Target.GPHome, conf.Target.CoordinatorPort(), filepath.Join(logArchiveDir+`\d{5}`, "data-migration-scripts"), idl.Step_finalize)
	expectedRegex := regexp.MustCompile(match)
	if !expectedRegex.MatchString(finalizeOutput) {
		t.Fatalf("expected %q to contain %v", finalizeOutput, expectedRegex)
	}

	// Ensure gpperfmon configuration file has been modified to reflect new
	// data directory location.
	contents := testutils.MustReadFile(t, filepath.Join(conf.Target.CoordinatorDataDir(), "gpperfmon", "conf", "gpperfmon.conf"))
	expected := fmt.Sprintf("log_location = %s", filepath.Join(conf.Target.CoordinatorDataDir(), "gpperfmon", "logs"))
	if !strings.Contains(contents, expected) {
		t.Error("Expected gpperfmon.conf to contain the target cluster coordinator data directory.")
		t.Errorf("expected %s to contain %q", contents, conf.Target.CoordinatorDataDir())
	}

	verifyPgHbaConfHostnames(t, source, conf.Target, useHbaHostnames)

	for _, host := range conf.Target.Hosts() {
		testutils.RemoteProcessMustNotBeRunning(t, host, "[g]pupgrade hub")
		testutils.RemoteProcessMustNotBeRunning(t, host, "[g]pupgrade agent")

		testutils.RemotePathMustNotExist(t, host, utils.GetStateDir())

		testutils.RemotePathMustExist(t, host, logArchiveDir+"*")
	}

	for _, seg := range conf.Target.Primaries {
		testutils.RemotePathMustExist(t, seg.Hostname, filepath.Join(seg.DataDir, "postgresql.conf"))
	}

	for _, seg := range conf.Target.Mirrors {
		testutils.RemotePathMustExist(t, seg.Hostname, filepath.Join(seg.DataDir, "postgresql.conf"))
	}

	// Check if old cluster was archived. We do this by taking the intermediate
	// cluster data directories appended with the .old suffix.
	for _, seg := range conf.Intermediate.Primaries {
		testutils.RemotePathMustExist(t, seg.Hostname, filepath.Join(seg.DataDir+upgrade.OldSuffix, "postgresql.conf"))
	}

	for _, seg := range conf.Intermediate.Mirrors {
		testutils.RemotePathMustExist(t, seg.Hostname, filepath.Join(seg.DataDir+upgrade.OldSuffix, "postgresql.conf"))
	}

	testutils.VerifyClusterIsRunning(t, *conf.Target)

	// verify configuration matches before and after upgrading
	compareFinalizedCluster(t, *conf.Target, acceptance.GetTargetCluster(t))

	err := conf.Target.WaitForClusterToBeReady()
	if err != nil {
		t.Fatal(err)
	}
}

// Usually we would simply use reflect.DeepEqual on the clusters. However, in
// copy mode the mirrors are upgraded using gpaddmirrors which can result in
// different dbid's for a given contentID depending on the order they are
// added. Thus, manually compare the clusters while ignoring the mirror dbid's.
func compareFinalizedCluster(t *testing.T, pre greenplum.Cluster, post greenplum.Cluster) {
	if pre.Destination != post.Destination {
		t.Errorf("got %v want %v", pre.Destination, post.Destination)
	}

	if !reflect.DeepEqual(pre.Primaries, post.Primaries) {
		t.Errorf("got %v want %v", pre.Primaries, post.Primaries)
	}

	for _, preMirror := range pre.Mirrors {
		postMirror := post.Mirrors[preMirror.ContentID]

		if preMirror.Port != postMirror.Port {
			t.Errorf("got %v want %v", preMirror.Port, postMirror.Port)
		}

		if preMirror.Hostname != postMirror.Hostname {
			t.Errorf("got %v want %v", preMirror.Hostname, postMirror.Hostname)
		}

		if preMirror.Address != postMirror.Address {
			t.Errorf("got %v want %v", preMirror.Address, postMirror.Address)
		}

		if preMirror.DataDir != postMirror.DataDir {
			t.Errorf("got %v want %v", preMirror.DataDir, postMirror.DataDir)
		}

		if preMirror.Role != postMirror.Role {
			t.Errorf("got %v want %v", preMirror.Role, postMirror.Role)
		}
	}

	if !reflect.DeepEqual(pre.Tablespaces, post.Tablespaces) {
		t.Errorf("got %v want %v", pre.Tablespaces, post.Tablespaces)
	}

	if pre.GPHome != post.GPHome {
		t.Errorf("got %v want %v", pre.GPHome, post.GPHome)
	}

	if !semver.Version.EQ(pre.Version, post.Version) {
		t.Errorf("got %v want %v", pre.Version, post.Version)
	}

	if pre.CatalogVersion != post.CatalogVersion {
		t.Errorf("got %v want %v", pre.CatalogVersion, post.CatalogVersion)
	}
}

func verifyPgHbaConfHostnames(t *testing.T, source greenplum.Cluster, target *greenplum.Cluster, useHbaHostnames bool) {
	t.Helper()

	expectation := "no hosts"
	var expected []string
	if useHbaHostnames {
		expectation = "all hosts"
		expected = source.Hosts()
	}

	for _, seg := range target.Primaries {
		actual := parsePgHbaConfForHosts(t, filepath.Join(seg.DataDir, "pg_hba.conf"))
		if !contains(actual, expected) {
			t.Errorf("expected pg_hba.conf of %s segment with contentID %d and dbID %d to contain %s", seg.Role, seg.ContentID, seg.DbID, expectation)
			t.Errorf("got %q want %q", actual, expected)
			t.Fatalf("pg_hba.conf: %s", testutils.MustReadFile(t, filepath.Join(seg.DataDir, "pg_hba.conf")))
		}
	}

	for _, seg := range target.Mirrors {
		actual := parsePgHbaConfForHosts(t, filepath.Join(seg.DataDir, "pg_hba.conf"))
		if !contains(actual, expected) {
			t.Errorf("expected pg_hba.conf of %s segment with contentID %d and dbID %d to contain %s", seg.Role, seg.ContentID, seg.DbID, expectation)
			t.Errorf("got %q want %q", actual, expected)
			t.Fatalf("pg_hba.conf: %s", testutils.MustReadFile(t, filepath.Join(seg.DataDir, "pg_hba.conf")))
		}
	}
}

// contains returns true if all needles are in the given slice
func contains(slice []string, needles []string) bool {
	sliceSet := make(map[string]bool)
	for _, elem := range slice {
		sliceSet[elem] = true
	}

	for _, needle := range needles {
		if !sliceSet[needle] {
			return false
		}
	}

	return true
}

func parsePgHbaConfForHosts(t *testing.T, path string) []string {
	t.Helper()

	uniqueHosts := make(map[string]bool)
	output := testutils.MustReadFile(t, path)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "host") {
			parts := strings.Fields(line)
			address := parts[3]
			uniqueHosts[address] = true
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scanning output: %v", err)
	}

	var hosts []string
	for host := range uniqueHosts {
		hosts = append(hosts, host)
	}

	sort.Strings(hosts)

	return hosts
}
