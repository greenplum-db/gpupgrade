// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"path/filepath"
	"testing"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
)

func Test_PgUpgrade_Upgradeable_Tests(t *testing.T) {
	// Since finalize archives the stateDir (GPUPGRADE_HOME) backups and
	// migration scripts cannot be stored here.
	stateDir := testutils.GetTempDir(t, "stateDir")
	defer testutils.MustRemoveAll(t, stateDir)

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

	source := GetSourceCluster(t)
	dir := "6-to-7"
	if source.Version.Major == 5 {
		dir = "5-to-6"
	}

	testDir := filepath.Join(MustGetRepoRoot(t), "test", "acceptance", dir)
	testutils.MustApplySQLFile(t, GPHOME_SOURCE, PGPORT, filepath.Join(testDir, "setup_globals.sql"))
	defer testutils.MustApplySQLFile(t, GPHOME_SOURCE, PGPORT, filepath.Join(testDir, "teardown_globals.sql"))

	t.Run("pg_upgrade upgradeable tests", func(t *testing.T) {
		sourceTestDir := filepath.Join(testDir, "upgradeable_tests", "source_cluster_regress")
		isolation2_regress(t, source.Version, GPHOME_SOURCE, PGPORT, sourceTestDir, sourceTestDir, idl.Schedule_upgradeable_source_schedule)

		initialize(t, idl.Mode_link)
		defer revert(t)
		execute(t)

		targetTestDir := filepath.Join(testDir, "upgradeable_tests", "target_cluster_regress")
		isolation2_regress(t, source.Version, GPHOME_TARGET, TARGET_PGPORT, targetTestDir, targetTestDir, idl.Schedule_upgradeable_target_schedule)
	})
}
