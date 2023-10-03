// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package pg_upgrade_test

import (
	"path/filepath"
	"testing"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/acceptance"
)

func Test_PgUpgrade_Upgradeable_Tests(t *testing.T) {
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

	source := acceptance.GetSourceCluster(t)
	dir := "6-to-7"
	if source.Version.Major == 5 {
		dir = "5-to-6"
	}

	testDir := filepath.Join(acceptance.MustGetRepoRoot(t), "test", "acceptance", dir)
	testutils.MustApplySQLFile(t, acceptance.GPHOME_SOURCE, acceptance.PGPORT, filepath.Join(testDir, "setup_globals.sql"))
	defer testutils.MustApplySQLFile(t, acceptance.GPHOME_SOURCE, acceptance.PGPORT, filepath.Join(testDir, "teardown_globals.sql"))

	t.Run("pg_upgrade upgradeable tests", func(t *testing.T) {
		acceptance.BackupDemoCluster(t, backupDir, source)
		defer acceptance.RestoreDemoCluster(t, backupDir, source, acceptance.GetTempTargetCluster(t))

		sourceTestDir := filepath.Join(testDir, "upgradeable_tests", "source_cluster_regress")
		acceptance.Isolation2_regress(t, source.Version, acceptance.GPHOME_SOURCE, acceptance.PGPORT, sourceTestDir, sourceTestDir, idl.Schedule_upgradeable_source_schedule)

		acceptance.Generate(t, migrationDir)
		acceptance.Apply(t, acceptance.GPHOME_SOURCE, acceptance.PGPORT, idl.Step_initialize, migrationDir)

		acceptance.Initialize(t, idl.Mode_link)
		defer acceptance.RevertIgnoreFailures(t)
		acceptance.Execute(t)
		acceptance.Finalize(t)

		acceptance.Apply(t, acceptance.GPHOME_TARGET, acceptance.PGPORT, idl.Step_finalize, migrationDir)

		targetTestDir := filepath.Join(testDir, "upgradeable_tests", "target_cluster_regress")
		acceptance.Isolation2_regress(t, source.Version, acceptance.GPHOME_SOURCE, acceptance.PGPORT, targetTestDir, targetTestDir, idl.Schedule_upgradeable_target_schedule)
	})
}
