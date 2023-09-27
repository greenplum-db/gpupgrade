// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/config"
	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

func TestPgUpgrade(t *testing.T) {
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

	t.Run("gpupgrade initialize runs pg_upgrade --check on coordinator and primaries", func(t *testing.T) {
		initialize(t, idl.Mode_copy)
		defer revert(t)

		logs := []string{
			testutils.MustGetLog(t, "hub"),
			MustGetPgUpgradeLog(t, -1),
			MustGetPgUpgradeLog(t, 0),
			MustGetPgUpgradeLog(t, 1),
			MustGetPgUpgradeLog(t, 2),
		}

		for _, log := range logs {
			contents := testutils.MustReadFile(t, log)
			expected := "Clusters are compatible"
			if !strings.Contains(contents, expected) {
				t.Errorf("expected %q to contain %q", contents, expected)
			}
		}
	})

	t.Run("upgrade maintains separate DBID for each segment and initialize runs gpinitsystem based on the source cluster", func(t *testing.T) {
		initialize(t, idl.Mode_copy)
		defer revert(t)

		execute(t)

		conf, err := config.Read()
		if err != nil {
			t.Fatal(err)
		}

		intermediate := GetIntermediateCluster(t)
		if len(source.Primaries) != len(intermediate.Primaries) {
			t.Fatalf("got %d want %d", len(source.Primaries), len(intermediate.Primaries))
		}

		segPrefix, err := greenplum.GetCoordinatorSegPrefix(source.CoordinatorDataDir())
		if err != nil {
			t.Fatal(err)
		}

		sourcePrimaries := source.SelectSegments(func(segConfig *greenplum.SegConfig) bool {
			return segConfig.IsPrimary() || segConfig.IsCoordinator()
		})

		sort.Sort(sourcePrimaries)

		expectedPort := 6020
		for _, sourcePrimary := range sourcePrimaries {
			intermediatePrimary := intermediate.Primaries[sourcePrimary.ContentID]

			if _, ok := intermediate.Primaries[sourcePrimary.ContentID]; !ok {
				t.Fatalf("source cluster primary segment with content id %d does not exist in the intermediate cluster", sourcePrimary.ContentID)
			}

			if sourcePrimary.DbID != intermediatePrimary.DbID {
				t.Errorf("got %d want %d", sourcePrimary.DbID, intermediatePrimary.DbID)
			}

			expectedDataDir := upgrade.TempDataDir(sourcePrimary.DataDir, segPrefix, conf.UpgradeID)
			if intermediatePrimary.DataDir != expectedDataDir {
				t.Errorf("got %q want %q", intermediatePrimary.DataDir, expectedDataDir)
			}

			if intermediatePrimary.Port != expectedPort {
				t.Errorf("got %d want %d", intermediatePrimary.Port, expectedPort)
			}

			expectedPort++
			if expectedPort == 6021 {
				// skip the standby port as the standby is created during finalize
				expectedPort++
			}
		}
	})

	t.Run("pg_upgrade upgradeable tests", func(t *testing.T) {
		sourceTestDir := filepath.Join(testDir, "upgradeable_tests", "source_cluster_regress")
		opts := isolationOptions{
			sourceVersion: source.Version,
			gphome:        GPHOME_SOURCE,
			port:          PGPORT,
			inputDir:      sourceTestDir,
			outputDir:     sourceTestDir,
			schedule:      "upgradeable_source_schedule",
			useExisting:   false,
		}
		isolation2_regress(t, opts)

		initialize(t, idl.Mode_link)
		defer revert(t)
		execute(t)

		targetTestDir := filepath.Join(testDir, "upgradeable_tests", "target_cluster_regress")
		opts = isolationOptions{
			sourceVersion: source.Version,
			gphome:        GPHOME_TARGET,
			port:          TARGET_PGPORT,
			inputDir:      targetTestDir,
			outputDir:     targetTestDir,
			schedule:      "upgradeable_target_schedule",
			useExisting:   true,
		}
		isolation2_regress(t, opts)
	})

	t.Run("pg_upgrade --check detects non-upgradeable objects", func(t *testing.T) {
		nonUpgradeableTestDir := filepath.Join(testDir, "non_upgradeable_tests")
		opts := isolationOptions{
			sourceVersion: source.Version,
			gphome:        GPHOME_SOURCE,
			port:          PGPORT,
			inputDir:      nonUpgradeableTestDir,
			outputDir:     nonUpgradeableTestDir,
			schedule:      "non_upgradeable_schedule",
			useExisting:   false,
		}
		isolation2_regress(t, opts)

		revert(t)
	})

}
