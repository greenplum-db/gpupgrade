// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
)

func TestPgUpgrade(t *testing.T) {
	stateDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, stateDir)

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

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

	t.Run("upgrade maintains separate DBID for each segment", func(t *testing.T) {
		source := GetSourceCluster(t)

		initialize(t, idl.Mode_copy)
		defer revert(t)

		execute(t)

		intermediate := GetIntermediateCluster(t)
		if len(source.Primaries) != len(intermediate.Primaries) {
			t.Fatalf("got %d want %d", len(source.Primaries), len(intermediate.Primaries))
		}

		for contentID := range source.Primaries {
			if _, ok := intermediate.Primaries[contentID]; !ok {
				t.Fatalf("source cluster primary segment with content id %d does not exist in the intermediate cluster", contentID)
			}

			if source.Primaries[contentID].DbID != intermediate.Primaries[contentID].DbID {
				t.Errorf("got %d want %d", source.Primaries[contentID].DbID, intermediate.Primaries[contentID].DbID)
			}
		}
	})
}
