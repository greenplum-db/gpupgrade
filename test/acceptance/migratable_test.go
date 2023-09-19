// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"path/filepath"
	"testing"

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
	defer testutils.MustRemoveAll(t, migrationDir)

	source := GetSourceCluster(t)
	dir := "6-to-7"
	if source.Version.Major == 5 {
		dir = "5-to-6"
	}

	testDir := filepath.Join(MustGetRepoRoot(t), "test", "acceptance", dir)
	migratableTestDir := filepath.Join(testDir, "migratable_tests")

	testutils.MustApplySQLFile(t, GPHOME_SOURCE, PGPORT, filepath.Join(testDir, "setup_globals.sql"))
	defer testutils.MustApplySQLFile(t, GPHOME_SOURCE, PGPORT, filepath.Join(testDir, "teardown_globals.sql"))

	t.Run("migration scripts generate sql to modify non-upgradeable objects and fix pg_upgrade check errors", func(t *testing.T) {
		backupDemoCluster(t, backupDir, source)
		defer restoreDemoCluster(t, backupDir, source, GetTempTargetCluster(t))

		sourceTestDir := filepath.Join(migratableTestDir, "source_cluster_regress")
		isolation2_regress(t, source.Version, GPHOME_SOURCE, PGPORT, sourceTestDir, "migratable_source_schedule")

		generate(t, migrationDir)
		apply(t, GPHOME_SOURCE, PGPORT, idl.Step_initialize, migrationDir)

		initialize(t, idl.Mode_link)
		execute(t)
		finalize(t)

		apply(t, GPHOME_TARGET, PGPORT, idl.Step_finalize, migrationDir)

		targetTestDir := filepath.Join(migratableTestDir, "target_cluster_regress")
		isolation2_regress(t, source.Version, GPHOME_TARGET, PGPORT, targetTestDir, "migratable_target_schedule")
	})
}
