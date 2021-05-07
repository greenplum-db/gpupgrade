// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"errors"
	"reflect"
	"testing"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

func TestAssignDataDirsAndPorts(t *testing.T) {
	var upgradeID upgrade.ID

	expectedDataDir := func(sourceDir string) string {
		return upgrade.TempDataDir(sourceDir, "seg", upgradeID)
	}

	cases := []struct {
		name string

		cluster  *greenplum.Cluster
		ports    []int
		expected InitializeConfig
	}{{
		name: "sorts and deduplicates provided port range",
		cluster: MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast2/seg2", Role: "p"},
		}),
		ports: []int{10, 9, 10, 9, 10, 8},
		expected: InitializeConfig{
			Master: greenplum.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: expectedDataDir("/data/qddir/seg-1"), Role: "p", Port: 8},
			Primaries: []greenplum.SegConfig{
				{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: expectedDataDir("/data/dbfast1/seg1"), Role: "p", Port: 9},
				{ContentID: 1, DbID: 3, Hostname: "mdw", DataDir: expectedDataDir("/data/dbfast2/seg2"), Role: "p", Port: 10},
			}},
	}, {
		name: "gives master its own port regardless of host layout",
		cluster: MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast2/seg2", Role: "p"},
			{ContentID: 2, DbID: 4, Hostname: "sdw1", DataDir: "/data/dbfast3/seg3", Role: "p"},
		}),
		ports: []int{50432, 50433, 50434, 50435, 50436, 50437, 50438, 50439, 50440},
		expected: InitializeConfig{
			Master: greenplum.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: expectedDataDir("/data/qddir/seg-1"), Role: "p", Port: 50432},
			Primaries: []greenplum.SegConfig{
				{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: expectedDataDir("/data/dbfast1/seg1"), Role: "p", Port: 50433},
				{ContentID: 1, DbID: 3, Hostname: "sdw1", DataDir: expectedDataDir("/data/dbfast2/seg2"), Role: "p", Port: 50434},
				{ContentID: 2, DbID: 4, Hostname: "sdw1", DataDir: expectedDataDir("/data/dbfast3/seg3"), Role: "p", Port: 50435},
			}},
	}, {
		name: "when using default ports, it sets up mirrors as expected in the InitializeConfig",
		cluster: MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast2/seg2", Role: "p"},
			{ContentID: 0, DbID: 4, Hostname: "sdw1", DataDir: "/data/dbfast_mirror1/seg1", Role: "m"},
			{ContentID: 1, DbID: 5, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg2", Role: "m"},
		}),
		ports: []int{50432, 50433, 50434, 50435, 50436, 50437, 50438, 50439, 50440},
		expected: InitializeConfig{
			Master: greenplum.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: expectedDataDir("/data/qddir/seg-1"), Role: "p", Port: 50432},
			Primaries: []greenplum.SegConfig{
				{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: expectedDataDir("/data/dbfast1/seg1"), Role: "p", Port: 50433},
				{ContentID: 1, DbID: 3, Hostname: "sdw1", DataDir: expectedDataDir("/data/dbfast2/seg2"), Role: "p", Port: 50434},
			},
			Mirrors: []greenplum.SegConfig{
				{ContentID: 0, DbID: 4, Hostname: "sdw1", DataDir: expectedDataDir("/data/dbfast_mirror1/seg1"), Role: "m", Port: 50435},
				{ContentID: 1, DbID: 5, Hostname: "sdw1", DataDir: expectedDataDir("/data/dbfast_mirror2/seg2"), Role: "m", Port: 50436},
			},
		},
	}, {
		name: "provides a standby port",
		cluster: MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: -1, DbID: 2, Hostname: "smdw", DataDir: "/data/standby", Role: "m"},
			{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
		}),
		ports: []int{50432, 50433, 50434, 50435, 50436, 50437, 50438, 50439, 50440},
		expected: InitializeConfig{
			Master:    greenplum.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: expectedDataDir("/data/qddir/seg-1"), Role: "p", Port: 50432},
			Standby:   greenplum.SegConfig{ContentID: -1, DbID: 2, Hostname: "smdw", DataDir: expectedDataDir("/data/standby"), Role: "m", Port: 50433},
			Primaries: []greenplum.SegConfig{{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: expectedDataDir("/data/dbfast1/seg1"), Role: "p", Port: 50434}},
		},
	}, {
		name: "deals with master and standby on the same host",
		cluster: MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: "/data/standby", Role: "m"},
			{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
		}),
		ports: []int{50432, 50433, 50434, 50435, 50436, 50437, 50438, 50439, 50440},
		expected: InitializeConfig{
			Master:    greenplum.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: expectedDataDir("/data/qddir/seg-1"), Role: "p", Port: 50432},
			Standby:   greenplum.SegConfig{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: expectedDataDir("/data/standby"), Role: "m", Port: 50433},
			Primaries: []greenplum.SegConfig{{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: expectedDataDir("/data/dbfast1/seg1"), Role: "p", Port: 50434}},
		},
	}, {
		name: "deals with master and standby on the same host as other segments",
		cluster: MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: "/data/standby", Role: "m"},
			{ContentID: 0, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: "p"},
		}),
		ports: []int{50432, 50433, 50434, 50435, 50436, 50437, 50438, 50439, 50440},
		expected: InitializeConfig{
			Master:    greenplum.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: expectedDataDir("/data/qddir/seg-1"), Role: "p", Port: 50432},
			Standby:   greenplum.SegConfig{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: expectedDataDir("/data/standby"), Role: "m", Port: 50433},
			Primaries: []greenplum.SegConfig{{ContentID: 0, DbID: 3, Hostname: "mdw", DataDir: expectedDataDir("/data/dbfast1/seg1"), Role: "p", Port: 50434}},
		},
	}, {
		name: "assigns provided ports to the standby",
		cluster: MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: "/data/standby", Role: "m"},
			{ContentID: 0, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: "p"},
		}),
		ports: []int{1, 2, 3},
		expected: InitializeConfig{
			Master:    greenplum.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: expectedDataDir("/data/qddir/seg-1"), Role: "p", Port: 1},
			Standby:   greenplum.SegConfig{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: expectedDataDir("/data/standby"), Role: "m", Port: 2},
			Primaries: []greenplum.SegConfig{{ContentID: 0, DbID: 3, Hostname: "mdw", DataDir: expectedDataDir("/data/dbfast1/seg1"), Role: "p", Port: 3}},
		},
	}, {
		name: "assigns provided ports to cluster with standby and multiple primaries and multiple mirrors",
		cluster: MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: "/data/standby", Role: "m"},
			{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 4, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Role: "p"},
			{ContentID: 2, DbID: 5, Hostname: "sdw3", DataDir: "/data/dbfast3/seg3", Role: "p"},
			{ContentID: 0, DbID: 6, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg1", Role: "m"},
			{ContentID: 1, DbID: 7, Hostname: "sdw3", DataDir: "/data/dbfast_mirror2/seg2", Role: "m"},
			{ContentID: 2, DbID: 8, Hostname: "sdw1", DataDir: "/data/dbfast_mirror3/seg3", Role: "m"},
		}),
		ports: []int{1, 2, 3, 4, 5},
		expected: InitializeConfig{
			Master:  greenplum.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: expectedDataDir("/data/qddir/seg-1"), Role: "p", Port: 1},
			Standby: greenplum.SegConfig{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: expectedDataDir("/data/standby"), Role: "m", Port: 2},
			Primaries: []greenplum.SegConfig{
				{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: expectedDataDir("/data/dbfast1/seg1"), Role: "p", Port: 3},
				{ContentID: 1, DbID: 4, Hostname: "sdw2", DataDir: expectedDataDir("/data/dbfast2/seg2"), Role: "p", Port: 3},
				{ContentID: 2, DbID: 5, Hostname: "sdw3", DataDir: expectedDataDir("/data/dbfast3/seg3"), Role: "p", Port: 3},
			},
			Mirrors: []greenplum.SegConfig{
				{ContentID: 0, DbID: 6, Hostname: "sdw2", DataDir: expectedDataDir("/data/dbfast_mirror1/seg1"), Role: "m", Port: 4},
				{ContentID: 1, DbID: 7, Hostname: "sdw3", DataDir: expectedDataDir("/data/dbfast_mirror2/seg2"), Role: "m", Port: 4},
				{ContentID: 2, DbID: 8, Hostname: "sdw1", DataDir: expectedDataDir("/data/dbfast_mirror3/seg3"), Role: "m", Port: 4},
			},
		},
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual, err := AssignDatadirsAndPorts(c.cluster, c.ports, upgradeID)
			if err != nil {
				t.Errorf("returned error %+v", err)
			}

			if !reflect.DeepEqual(actual, c.expected) {
				t.Errorf("AssignDatadirsAndPorts(<cluster>, %v)=%v, want %v", c.ports, actual, c.expected)
			}
		})
	}

	errCases := []struct {
		name string

		cluster *greenplum.Cluster
		ports   []int
	}{{
		name: "errors when not given enough ports (single host)",
		cluster: MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast2/seg2", Role: "p"},
		}),
		ports: []int{15433},
	}, {
		name: "errors when not given enough ports (multiple hosts)",
		cluster: MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast2/seg2", Role: "p"},
		}),
		ports: []int{15433, 25432},
	}, {
		name: "errors when not given enough unique ports",
		cluster: MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast2/seg2", Role: "p"},
		}),
		ports: []int{15433, 15433, 15433},
	}, {
		name: "errors when not given enough unique ports with a standby",
		cluster: MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "m"},
			{ContentID: 0, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: "p"},
		}),
		ports: []int{15433, 15434},
	}, {
		name: "errors when not given enough unique ports with a standby on a different host",
		cluster: MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: -1, DbID: 2, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: "m"},
			{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
		}),
		ports: []int{15433, 15434},
	}, {
		name: "errors when there are not enough ports for the mirrors",
		cluster: MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast_mirror1/seg1", Role: "m"},
		}),
		ports: []int{15433, 15434},
	}, {
		// regression case
		name: "doesn't panic when not given enough unique ports with a standby on a different host",
		cluster: MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: -1, DbID: 2, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: "m"},
			{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
		}),
		ports: []int{15433},
	}}

	for _, c := range errCases {
		t.Run(c.name, func(t *testing.T) {
			_, err := AssignDatadirsAndPorts(c.cluster, c.ports, 0)
			if err == nil {
				t.Errorf("AssignDatadirsAndPorts(<cluster>, %v) returned nil, want error", c.ports)
			}
		})
	}
}

func TestEnsureTempPortRangeDoesNotOverlapWithSourceClusterPorts(t *testing.T) {
	source := MustCreateCluster(t, []greenplum.SegConfig{
		{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", Port: 15432},
		{ContentID: -1, DbID: 8, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: "m", Port: 16432},
		{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg0", Role: "p", Port: 25432},
		{ContentID: 0, DbID: 5, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg0", Role: "m", Port: 25435},
	})

	target := InitializeConfig{
		Master:  greenplum.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", Port: 6000},
		Standby: greenplum.SegConfig{ContentID: -1, DbID: 8, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: "m", Port: 6001},
		Primaries: []greenplum.SegConfig{
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg0", Role: "p", Port: 6002},
		},
		Mirrors: []greenplum.SegConfig{
			{ContentID: 0, DbID: 5, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg0", Role: "m", Port: 6005},
		}}

	t.Run("ensureTempPortRangeDoesNotOverlapWithSourceClusterPorts succeeds", func(t *testing.T) {
		err := ensureTempPortRangeDoesNotOverlapWithSourceClusterPorts(source, target)
		if err != nil {
			t.Errorf("expected error %#v got nil", err)
		}
	})

	errCases := []struct {
		name            string
		source          *greenplum.Cluster
		target          InitializeConfig
		conflictingPort int
	}{{
		name: "errors when source master port overlaps with temp target cluster ports",
		source: MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", Port: 15432},
			{ContentID: -1, DbID: 8, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: "m", Port: 16432},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg0", Role: "p", Port: 25432},
			{ContentID: 0, DbID: 5, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg0", Role: "m", Port: 25435},
		}),
		target: InitializeConfig{
			Master:  greenplum.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", Port: 15432},
			Standby: greenplum.SegConfig{ContentID: -1, DbID: 8, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: "m", Port: 6001},
			Primaries: []greenplum.SegConfig{
				{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg0", Role: "p", Port: 6002},
			},
			Mirrors: []greenplum.SegConfig{
				{ContentID: 0, DbID: 5, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg0", Role: "m", Port: 6005},
			}},
		conflictingPort: 15432,
	}, {
		name: "errors when source standby port overlaps with temp target cluster ports",
		source: MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", Port: 15432},
			{ContentID: -1, DbID: 8, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: "m", Port: 16432},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg0", Role: "p", Port: 25432},
			{ContentID: 0, DbID: 5, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg0", Role: "m", Port: 25435},
		}),
		target: InitializeConfig{
			Master:  greenplum.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", Port: 6000},
			Standby: greenplum.SegConfig{ContentID: -1, DbID: 8, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: "m", Port: 16432},
			Primaries: []greenplum.SegConfig{
				{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg0", Role: "p", Port: 6002},
			},
			Mirrors: []greenplum.SegConfig{
				{ContentID: 0, DbID: 5, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg0", Role: "m", Port: 6005},
			}},
		conflictingPort: 16432,
	}, {
		name: "errors when source primary port overlaps with temp target cluster ports",
		source: MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", Port: 15432},
			{ContentID: -1, DbID: 8, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: "m", Port: 16432},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg0", Role: "p", Port: 25432},
			{ContentID: 0, DbID: 5, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg0", Role: "m", Port: 25435},
		}),
		target: InitializeConfig{
			Master:  greenplum.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", Port: 6000},
			Standby: greenplum.SegConfig{ContentID: -1, DbID: 8, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: "m", Port: 6001},
			Primaries: []greenplum.SegConfig{
				{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg0", Role: "p", Port: 25432},
			},
			Mirrors: []greenplum.SegConfig{
				{ContentID: 0, DbID: 5, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg0", Role: "m", Port: 6005},
			}},
		conflictingPort: 25432,
	}, {
		name: "errors when source mirror port overlaps with temp target cluster ports",
		source: MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", Port: 15432},
			{ContentID: -1, DbID: 8, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: "m", Port: 16432},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg0", Role: "p", Port: 25432},
			{ContentID: 0, DbID: 5, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg0", Role: "m", Port: 25435},
		}),
		target: InitializeConfig{
			Master:  greenplum.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", Port: 6000},
			Standby: greenplum.SegConfig{ContentID: -1, DbID: 8, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: "m", Port: 6001},
			Primaries: []greenplum.SegConfig{
				{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg0", Role: "p", Port: 6002},
			},
			Mirrors: []greenplum.SegConfig{
				{ContentID: 0, DbID: 5, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg0", Role: "m", Port: 25435},
			}},
		conflictingPort: 25435,
	}}

	for _, c := range errCases {
		t.Run(c.name, func(t *testing.T) {
			err := ensureTempPortRangeDoesNotOverlapWithSourceClusterPorts(c.source, c.target)
			var invalidPortErr *InvalidTempPortRangeError
			if !errors.As(err, &invalidPortErr) {
				t.Fatalf("got %T, want %T", err, invalidPortErr)
			}

			if invalidPortErr.conflictingPort != c.conflictingPort {
				t.Errorf("got conflicting port %d, want %d", invalidPortErr.conflictingPort, c.conflictingPort)
			}
		})
	}
}
