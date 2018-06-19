package testutils

import (
	"fmt"
	"net"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/hub/services"
)

const (
	MASTER_ONLY_JSON = `{
	"SegConfig": [{
		"address": "briarwood",
		"content": -1,
		"datadir": "/old/datadir",
		"dbid": 1,
		"hostname": "briarwood",
		"mode": "s",
		"port": 25437,
		"preferred_role": "m",
		"role": "m",
		"san_mounts": null,
		"status": "u"
	}],
	"BinDir": "/old/tmp"}
`

	NEW_MASTER_JSON = `{
	"SegConfig": [{
		"address": "aspen",
		"content": -1,
		"datadir": "/new/datadir",
		"dbid": 1,
		"hostname": "briarwood",
		"mode": "s",
		"port": 35437,
		"preferred_role": "m",
		"role": "m",
		"san_mounts": null,
		"status": "u"
	}],
	"BinDir": "/new/tmp"}
`
)

func Check(msg string, e error) {
	if e != nil {
		panic(fmt.Sprintf("%s: %s\n", msg, e.Error()))
	}
}

func CreateSampleCluster(contentID int, port int, hostname string, datadir string) *cluster.Cluster {
	return &cluster.Cluster{
		ContentIDs: []int{contentID},
		Segments: map[int]cluster.SegConfig{
			contentID: cluster.SegConfig{ContentID: contentID, Port: port, Hostname: hostname, DataDir: datadir},
		},
	}
}

func CreateSampleClusterPair() *services.ClusterPair {
	cp := &services.ClusterPair{
		OldCluster: CreateSampleCluster(-1, 25437, "hostone", "/old/datadir"),
		NewCluster: CreateSampleCluster(-1, 35437, "", "/new/datadir"),
	}
	cp.OldCluster.Executor = &testhelper.TestExecutor{}
	cp.NewCluster.Executor = &testhelper.TestExecutor{}
	cp.OldBinDir = "/old/bindir"
	cp.NewBinDir = "/new/bindir"
	return cp
}

func InitClusterPairFromDB() *services.ClusterPair {
	conn := dbconn.NewDBConnFromEnvironment("postgres")
	conn.MustConnect(1)
	conn.Version.Initialize(conn)
	cp := &services.ClusterPair{}
	segConfig := cluster.MustGetSegmentConfiguration(conn)
	cp.OldCluster = cluster.NewCluster(segConfig)
	cp.OldBinDir = "/old/bindir"
	cp.NewCluster = cluster.NewCluster(segConfig)
	cp.NewBinDir = "/new/bindir"
	return cp
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
