// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
)

func TestMigrationScripts(t *testing.T) {
	// Since finalize archives the stateDir (GPUPGRADE_HOME) backups and
	// migration scripts cannot be stored here.
	stateDir := testutils.GetTempDir(t, "stateDir")
	defer testutils.MustRemoveAll(t, stateDir)

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

	backupDir := testutils.GetTempDir(t, "backup")
	defer testutils.MustRemoveAll(t, backupDir)

	migrationDir := testutils.GetTempDir(t, "migration")
	defer testutils.MustRemoveAll(t, backupDir)

	source := GetSourceCluster(t)

	dir := "6-to-7"
	if source.Version.Major == 5 {
		dir = "5-to-6"
	}

	testDir := filepath.Join(MustGetRepoRoot(t), "test", "acceptance", dir, "migratable_tests")

	t.Run("migration scripts generate sql to modify non-upgradeable objects and fix pg_upgrade check errors", func(t *testing.T) {
		backupDemoCluster(t, backupDir, source)
		defer restoreDemoCluster(t, backupDir, source, GetTempTargetCluster(t))

		// Remove any default non-upgradeable objects such as GPDB 5X gphdfs role.
		generate(t, migrationDir)
		apply(t, GPHOME_SOURCE, PGPORT, idl.Step_initialize, migrationDir)

		testutils.MustApplySQLFile(t, GPHOME_SOURCE, PGPORT, filepath.Join(testDir, "setup_migratable_globals.sql"))
		defer testutils.MustApplySQLFile(t, GPHOME_SOURCE, PGPORT, filepath.Join(testDir, "teardown_migratable_globals.sql"))

		source_isolation2_regress(t, source.Version, testDir, "migratable_source_schedule")

		generate(t, migrationDir)
		apply(t, GPHOME_SOURCE, PGPORT, idl.Step_initialize, migrationDir)

		initialize(t, idl.Mode_link)
		defer revertIgnoreFailures(t) // cleanup in case we fail part way through
		execute(t)
		finalize(t)

		apply(t, GPHOME_TARGET, PGPORT, idl.Step_finalize, migrationDir)

		target_isolation2_regress(t, source.Version, testDir, "migratable_source_schedule")
	})
}

func source_isolation2_regress(t *testing.T, sourceVersion semver.Version, testDir string, schedule string) string {
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

	tests := "--schedule=" + filepath.Join(testDir, "source_cluster_regress", schedule)
	focus := os.Getenv("FOCUS_TESTS")
	if focus != "" {
		tests = focus
	}

	cmd := exec.Command("./pg_isolation2_regress",
		"--init-file", "init_file_isolation2",
		"--inputdir", filepath.Join(testDir, "source_cluster_regress"),
		"--outputdir", filepath.Join(testDir, "source_cluster_regress"),
		binDir, filepath.Join(GPHOME_SOURCE, "bin"),
		"--port", PGPORT,
		tests)
	cmd.Dir = testutils.MustGetEnv("ISOLATION2_PATH")
	cmd.Env = append(os.Environ(), env...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %#v stderr %s", err, output)
	}

	return strings.TrimSpace(string(output))
}

func target_isolation2_regress(t *testing.T, sourceVersion semver.Version, testDir string, schedule string) string {
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

	tests := "--schedule=" + filepath.Join(testDir, "source_cluster_regress", schedule)
	focus := os.Getenv("FOCUS_TESTS")
	if focus != "" {
		tests = focus
	}

	cmd := exec.Command("./pg_isolation2_regress",
		"--init-file", "init_file_isolation2",
		"--inputdir", filepath.Join(testDir, "target_cluster_regress"),
		"--outputdir", filepath.Join(testDir, "target_cluster_regress"),
		binDir, filepath.Join(GPHOME_TARGET, "bin"),
		"--port", PGPORT,
		"--use-existing",
		tests)
	cmd.Dir = testutils.MustGetEnv("ISOLATION2_PATH")
	cmd.Env = append(os.Environ(), env...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %#v stderr %s", err, output)
	}

	return strings.TrimSpace(string(output))
}
