// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/testutils"
)

func TestWriteAddMirrorsConfig(t *testing.T) {
	t.Run("writes gpaddmirrors_config", func(t *testing.T) {
		stateDir := testutils.GetTempDir(t, "")
		defer os.RemoveAll(stateDir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
		defer resetEnv()

		intermediate := MustCreateCluster(t, greenplum.SegConfigs{
			{DbID: 1, ContentID: -1, Hostname: "coordinator", DataDir: "/data/qddir/seg.HqtFHX54y0o.-1", Port: 50432, Role: greenplum.PrimaryRole},
			{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby.HqtFHX54y0o", Port: 50433, Role: greenplum.MirrorRole},
			{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg.HqtFHX54y0o.1", Port: 50434, Role: greenplum.PrimaryRole},
			{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg.HqtFHX54y0o.1", Port: 50435, Role: greenplum.MirrorRole},
			{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg.HqtFHX54y0o.2", Port: 50436, Role: greenplum.PrimaryRole},
			{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg.HqtFHX54y0o.2", Port: 50437, Role: greenplum.MirrorRole},
		})

		config, err := writeAddMirrorsConfig(intermediate)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		// iterating maps is not deterministic so sort and rejoin before asserting
		lines := strings.Split(testutils.MustReadFile(t, config), "\n")
		sort.Strings(lines)
		actual := strings.Join(lines, "\n")

		expected := `
0|sdw2|50435|/data/dbfast_mirror1/seg.HqtFHX54y0o.1
1|sdw1|50437|/data/dbfast_mirror2/seg.HqtFHX54y0o.2`
		if actual != expected {
			t.Errorf("got %q want %q", actual, expected)
		}
	})
}
