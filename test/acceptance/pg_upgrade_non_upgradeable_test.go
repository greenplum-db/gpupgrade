// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"path/filepath"
	"testing"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/acceptance"
)

func Test_PgUpgrade_NonUpgradeable_Tests(t *testing.T) {
	// Since finalize archives the stateDir (GPUPGRADE_HOME) backups and
	// migration scripts cannot be stored here.
	stateDir := testutils.GetTempDir(t, "stateDir")
	defer testutils.MustRemoveAll(t, stateDir)

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

	source := acceptance.GetSourceCluster(t)
	dir := "6-to-7"
	if source.Version.Major == 5 {
		dir = "5-to-6"
	}

	testDir := filepath.Join(acceptance.MustGetRepoRoot(t), "test", "acceptance", dir)
	testutils.MustApplySQLFile(t, acceptance.GPHOME_SOURCE, acceptance.PGPORT, filepath.Join(testDir, "setup_globals.sql"))
	defer testutils.MustApplySQLFile(t, acceptance.GPHOME_SOURCE, acceptance.PGPORT, filepath.Join(testDir, "teardown_globals.sql"))

	t.Run("pg_upgrade --check detects non-upgradeable objects", func(t *testing.T) {
		nonUpgradeableTestDir := filepath.Join(testDir, "non_upgradeable_tests")
		acceptance.Isolation2_regress(t, source.Version, acceptance.GPHOME_SOURCE, acceptance.PGPORT, nonUpgradeableTestDir, nonUpgradeableTestDir, idl.Schedule_non_upgradeable_schedule)

		acceptance.Revert(t)
	})

}
