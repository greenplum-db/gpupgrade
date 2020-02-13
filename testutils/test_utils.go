package testutils

import (
	"net"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"

	"github.com/greenplum-db/gpupgrade/utils"
)

func CreateMultinodeSampleCluster(baseDir string) *utils.Cluster {
	return &utils.Cluster{
		ContentIDs: []int{-1, 0, 1},
		Primaries: map[int]utils.SegConfig{
			-1: {ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: baseDir + "/seg-1"},
			0:  {ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: baseDir + "/seg1"},
			1:  {ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: baseDir + "/seg2"},
		},
	}
}

func CreateMultinodeSampleClusterPair(baseDir string) (*utils.Cluster, *utils.Cluster) {
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
