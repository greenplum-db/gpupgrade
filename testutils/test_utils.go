package testutils

import (
	"net"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"

	"github.com/greenplum-db/gpupgrade/greenplum"
)

// TODO remove in favor of MustCreateCluster
func CreateMultinodeSampleCluster(baseDir string) *greenplum.Cluster {
	return &greenplum.Cluster{
		ContentIDs: []int{-1, 0, 1},
		Primaries: map[int]greenplum.SegConfig{
			-1: {ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: baseDir + "/seg-1", Role: "p"},
			0:  {ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: baseDir + "/seg1", Role: "p"},
			1:  {ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: baseDir + "/seg2", Role: "p"},
		},
	}
}

// TODO remove in favor of MustCreateCluster
func CreateMultinodeSampleClusterPair(baseDir string) (*greenplum.Cluster, *greenplum.Cluster) {
	gpdbVersion := dbconn.NewVersion("6.0.0")

	sourceCluster := CreateMultinodeSampleCluster(baseDir)
	sourceCluster.BinDir = "/source/bindir"
	sourceCluster.Version = gpdbVersion

	targetCluster := CreateMultinodeSampleCluster(baseDir)
	targetCluster.BinDir = "/target/bindir"
	targetCluster.Version = gpdbVersion

	return sourceCluster, targetCluster
}

func GetOpenPort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()

	return l.Addr().(*net.TCPAddr).Port, nil
}
