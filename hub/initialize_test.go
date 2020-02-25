package hub

import (
	"reflect"
	"testing"

	"github.com/greenplum-db/gpupgrade/utils"
)

func TestAssignPorts(t *testing.T) {

	cases := []struct {
		name string

		cluster  *utils.Cluster
		ports    []int
		expected InitializeConfig
	}{{
		name: "sorts and deduplicates provided port range",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast2/seg2", Role: "p"},
		}),
		ports: []int{10, 9, 10, 9, 10, 8},
		expected: InitializeConfig{
			Master: utils.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir_upgrade/seg-1", Role: "p", Port: 8},
			Primaries: []utils.SegConfig{
				{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: "/data/dbfast1_upgrade/seg1", Role: "p", Port: 9},
				{ContentID: 1, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast2_upgrade/seg2", Role: "p", Port: 10},
			}},
	}, {
		name: "uses default port range when port list is empty",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast2/seg2", Role: "p"},
			{ContentID: 2, DbID: 4, Hostname: "sdw1", DataDir: "/data/dbfast3/seg3", Role: "p"},
		}),
		ports: []int{},
		expected: InitializeConfig{
			Master: utils.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir_upgrade/seg-1", Role: "p", Port: 50432},
			Primaries: []utils.SegConfig{
				{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: "/data/dbfast1_upgrade/seg1", Role: "p", Port: 50433},
				{ContentID: 1, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast2_upgrade/seg2", Role: "p", Port: 50434},
				{ContentID: 2, DbID: 4, Hostname: "sdw1", DataDir: "/data/dbfast3_upgrade/seg3", Role: "p", Port: 50433},
			}},
	}, {
		name: "gives master its own port regardless of host layout",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast2/seg2", Role: "p"},
			{ContentID: 2, DbID: 4, Hostname: "sdw1", DataDir: "/data/dbfast3/seg3", Role: "p"},
		}),
		ports: []int{},
		expected: InitializeConfig{
			Master: utils.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir_upgrade/seg-1", Role: "p", Port: 50432},
			Primaries: []utils.SegConfig{
				{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1_upgrade/seg1", Role: "p", Port: 50433},
				{ContentID: 1, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast2_upgrade/seg2", Role: "p", Port: 50434},
				{ContentID: 2, DbID: 4, Hostname: "sdw1", DataDir: "/data/dbfast3_upgrade/seg3", Role: "p", Port: 50435},
			}},
	}, {
		name: "provides a standby port",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: -1, DbID: 2, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: "m"},
			{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
		}),
		ports: []int{},
		expected: InitializeConfig{
			Master:    utils.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir_upgrade/seg-1", Role: "p", Port: 50432},
			Standby:   utils.SegConfig{ContentID: -1, DbID: 2, Hostname: "smdw", DataDir: "/data/qddir_upgrade/seg-1", Role: "m", Port: 50433},
			Primaries: []utils.SegConfig{{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast1_upgrade/seg1", Role: "p", Port: 50434}},
		},
	}, {
		name: "deals with master and standby on the same host",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "m"},
			{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
		}),
		ports: []int{},
		expected: InitializeConfig{
			Master:    utils.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir_upgrade/seg-1", Role: "p", Port: 50432},
			Standby:   utils.SegConfig{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: "/data/qddir_upgrade/seg-1", Role: "m", Port: 50433},
			Primaries: []utils.SegConfig{{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast1_upgrade/seg1", Role: "p", Port: 50434}},
		},
	}, {
		name: "deals with master and standby on the same host as other segments",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "m"},
			{ContentID: 0, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: "p"},
		}),
		ports: []int{},
		expected: InitializeConfig{
			Master:    utils.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir_upgrade/seg-1", Role: "p", Port: 50432},
			Standby:   utils.SegConfig{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: "/data/qddir_upgrade/seg-1", Role: "m", Port: 50433},
			Primaries: []utils.SegConfig{{ContentID: 0, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast1_upgrade/seg1", Role: "p", Port: 50434}},
		},
	}, {
		name: "assigns provided ports to the standby",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "m"},
			{ContentID: 0, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: "p"},
		}),
		ports: []int{1, 2, 3},
		expected: InitializeConfig{
			Master:    utils.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir_upgrade/seg-1", Role: "p", Port: 1},
			Standby:   utils.SegConfig{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: "/data/qddir_upgrade/seg-1", Role: "m", Port: 2},
			Primaries: []utils.SegConfig{{ContentID: 0, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast1_upgrade/seg1", Role: "p", Port: 3}},
		},
	}, {
		name: "assigns provided ports to cluster with standby and multiple primaries and multiple mirrors",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "m"},
			{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 4, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Role: "p"},
			{ContentID: 2, DbID: 5, Hostname: "sdw3", DataDir: "/data/dbfast3/seg3", Role: "p"},
			{ContentID: 0, DbID: 6, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg1", Role: "m"},
			{ContentID: 1, DbID: 7, Hostname: "sdw3", DataDir: "/data/dbfast_mirror2/seg2", Role: "m"},
			{ContentID: 2, DbID: 8, Hostname: "sdw1", DataDir: "/data/dbfast_mirror3/seg3", Role: "m"},
		}),
		ports: []int{1, 2, 3, 4, 5},
		expected: InitializeConfig{
			Master:  utils.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir_upgrade/seg-1", Role: "p", Port: 1},
			Standby: utils.SegConfig{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: "/data/qddir_upgrade/seg-1", Role: "m", Port: 2},
			Primaries: []utils.SegConfig{
				{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast1_upgrade/seg1", Role: "p", Port: 3},
				{ContentID: 1, DbID: 4, Hostname: "sdw2", DataDir: "/data/dbfast2_upgrade/seg2", Role: "p", Port: 3},
				{ContentID: 2, DbID: 5, Hostname: "sdw3", DataDir: "/data/dbfast3_upgrade/seg3", Role: "p", Port: 3},
			},
		},
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual, err := AssignDatadirsAndPorts(c.cluster, c.ports)
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

		cluster *utils.Cluster
		ports   []int
	}{{
		name: "errors when not given enough ports (single host)",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast2/seg2", Role: "p"},
		}),
		ports: []int{15433},
	}, {
		name: "errors when not given enough ports (multiple hosts)",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast2/seg2", Role: "p"},
		}),
		ports: []int{15433, 25432},
	}, {
		name: "errors when not given enough unique ports",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast2/seg2", Role: "p"},
		}),
		ports: []int{15433, 15433, 15433},
	}, {
		name: "errors when not given enough unique ports with a standby",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "m"},
			{ContentID: 0, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: "p"},
		}),
		ports: []int{15433, 15434},
	}, {
		name: "errors when not given enough unique ports with a standby on a different host",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: -1, DbID: 2, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: "m"},
			{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
		}),
		ports: []int{15433, 15434},
	}, {
		// regression case
		name: "doesn't panic when not given enough unique ports with a standby on a different host",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: -1, DbID: 2, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: "m"},
			{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
		}),
		ports: []int{15433},
	}}

	for _, c := range errCases {
		t.Run(c.name, func(t *testing.T) {
			_, err := AssignDatadirsAndPorts(c.cluster, c.ports)
			if err == nil {
				t.Errorf("AssignDatadirsAndPorts(<cluster>, %v) returned nil, want error", c.ports)
			}
		})
	}
}
