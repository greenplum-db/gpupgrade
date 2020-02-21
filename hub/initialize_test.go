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
		expected PortAssignments
	}{{
		name:     "sorts and deduplicates provided port range",
		cluster:  MustCreateCluster(t, []utils.SegConfig{}),
		ports:    []int{10, 9, 10, 9, 10, 8},
		expected: PortAssignments{8, 0, []int{9, 10}},
	}, {
		name: "uses default port range when port list is empty",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", PreferredRole: "p"},
			{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: "p", PreferredRole: "p"},
			{ContentID: 1, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast2/seg2", Role: "p", PreferredRole: "p"},
			{ContentID: 2, DbID: 4, Hostname: "sdw1", DataDir: "/data/dbfast3/seg3", Role: "p", PreferredRole: "p"},
		}),
		ports:    []int{},
		expected: PortAssignments{50432, 0, []int{50433, 50434}},
	}, {
		name: "gives master its own port regardless of host layout",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", PreferredRole: "p"},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p", PreferredRole: "p"},
			{ContentID: 1, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast2/seg2", Role: "p", PreferredRole: "p"},
			{ContentID: 2, DbID: 4, Hostname: "sdw1", DataDir: "/data/dbfast3/seg3", Role: "p", PreferredRole: "p"},
		}),
		ports:    []int{},
		expected: PortAssignments{50432, 0, []int{50433, 50434, 50435}},
	}, {
		name: "provides a standby port",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", PreferredRole: "p"},
			{ContentID: -1, DbID: 2, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: "m", PreferredRole: "m"},
			{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p", PreferredRole: "p"},
		}),
		ports:    []int{},
		expected: PortAssignments{50432, 50433, []int{50434}},
	}, {
		name: "deals with master and standby on the same host",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", PreferredRole: "p"},
			{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "m", PreferredRole: "m"},
			{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p", PreferredRole: "p"},
		}),
		ports:    []int{},
		expected: PortAssignments{50432, 50433, []int{50434}},
	}, {
		name: "deals with master and standby on the same host as other segments",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", PreferredRole: "p"},
			{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "m", PreferredRole: "m"},
			{ContentID: 0, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: "p", PreferredRole: "p"},
		}),
		ports:    []int{},
		expected: PortAssignments{50432, 50433, []int{50434}},
	}, {
		name: "assigns provided ports to the standby",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", PreferredRole: "p"},
			{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "m", PreferredRole: "m"},
			{ContentID: 0, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: "p", PreferredRole: "p"},
		}),
		ports:    []int{1, 2, 3},
		expected: PortAssignments{1, 2, []int{3}},
	}, {
		name: "assigns provided ports to cluster with standby and multiple primaries",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", PreferredRole: "p"},
			{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "m", PreferredRole: "m"},
			{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p", PreferredRole: "p"},
			{ContentID: 1, DbID: 4, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Role: "p", PreferredRole: "p"},
			{ContentID: 2, DbID: 5, Hostname: "sdw3", DataDir: "/data/dbfast3/seg3", Role: "p", PreferredRole: "p"},
		}),
		ports:    []int{1, 2, 3, 4, 5},
		expected: PortAssignments{1, 2, []int{3, 4, 5}},
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual, err := assignPorts(c.cluster, c.ports)
			if err != nil {
				t.Errorf("returned error %+v", err)
			}

			if !reflect.DeepEqual(actual, c.expected) {
				t.Errorf("assignPorts(<cluster>, %v)=%v, want %v", c.ports, actual, c.expected)
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
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", PreferredRole: "p"},
			{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: "p", PreferredRole: "p"},
			{ContentID: 1, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast2/seg2", Role: "p", PreferredRole: "p"},
		}),
		ports: []int{15433},
	}, {
		name: "errors when not given enough ports (multiple hosts)",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", PreferredRole: "p"},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p", PreferredRole: "p"},
			{ContentID: 1, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast2/seg2", Role: "p", PreferredRole: "p"},
		}),
		ports: []int{15433, 25432},
	}, {
		name: "errors when not given enough unique ports",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", PreferredRole: "p"},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p", PreferredRole: "p"},
			{ContentID: 1, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast2/seg2", Role: "p", PreferredRole: "p"},
		}),
		ports: []int{15433, 15433, 15433},
	}, {
		name: "errors when not given enough unique ports with a standby",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", PreferredRole: "p"},
			{ContentID: -1, DbID: 2, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "m", PreferredRole: "m"},
			{ContentID: 0, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: "p", PreferredRole: "p"},
		}),
		ports: []int{15433, 15434},
	}, {
		name: "errors when not given enough unique ports with a standby on a different host",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", PreferredRole: "p"},
			{ContentID: -1, DbID: 2, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: "m", PreferredRole: "m"},
			{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p", PreferredRole: "p"},
		}),
		ports: []int{15433, 15434},
	}, {
		// regression case
		name: "doesn't panic when not given enough unique ports with a standby on a different host",
		cluster: MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p", PreferredRole: "p"},
			{ContentID: -1, DbID: 2, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: "m", PreferredRole: "m"},
			{ContentID: 0, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p", PreferredRole: "p"},
		}),
		ports: []int{15433},
	}}

	for _, c := range errCases {
		t.Run(c.name, func(t *testing.T) {
			_, err := assignPorts(c.cluster, c.ports)
			if err == nil {
				t.Errorf("assignPorts(<cluster>, %v) returned nil, want error", c.ports)
			}
		})
	}

	t.Run("checkTargetPorts panics if it is passed zero ports", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("checkTargetPorts did not panic")
			}
		}()

		checkTargetPorts(MustCreateCluster(t, []utils.SegConfig{}), []int{})
	})
}
