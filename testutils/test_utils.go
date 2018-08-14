package testutils

import (
	"fmt"
	"net"
	"path/filepath"

	"github.com/blang/semver"
	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/utils"
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

func CreateMultinodeSampleCluster(baseDir string) *cluster.Cluster {
	return &cluster.Cluster{
		ContentIDs: []int{-1, 0, 1},
		Segments: map[int]cluster.SegConfig{
			-1: cluster.SegConfig{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: baseDir + "/seg-1"},
			0:  cluster.SegConfig{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: baseDir + "/seg1"},
			1:  cluster.SegConfig{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: baseDir + "/seg2"},
		},
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

func CreateMultinodeSampleClusterPair(baseDir string) (*utils.Cluster, *utils.Cluster) {
	sourceCluster := CreateMultinodeSampleCluster(baseDir)
	targetCluster := CreateMultinodeSampleCluster(baseDir)
	return assembleClusters(baseDir, sourceCluster, targetCluster)
}

func CreateSampleClusterPair() (*utils.Cluster, *utils.Cluster) {
	sourceCluster := CreateSampleCluster(-1, 25437, "hostone", "/source/datadir")
	targetCluster := CreateSampleCluster(-1, 35437, "hosttwo", "/target/datadir")
	return assembleClusters("/tmp", sourceCluster, targetCluster)
}

func InitClusterPairFromDB() (*utils.Cluster, *utils.Cluster) {
	conn := dbconn.NewDBConnFromEnvironment("postgres")
	conn.MustConnect(1)
	conn.Version.Initialize(conn)
	segConfig := cluster.MustGetSegmentConfiguration(conn)
	sourceCluster := cluster.NewCluster(segConfig)
	targetCluster := cluster.NewCluster(segConfig)
	return assembleClusters("/tmp", sourceCluster, targetCluster)
}

func assembleClusters(baseDir string, sourceCluster *cluster.Cluster, targetCluster *cluster.Cluster) (source *utils.Cluster, target *utils.Cluster) {
	version, err := semver.Make("6.0.0")
	if err != nil {
		panic(err)
	}

	gpdbVersion := dbconn.GPDBVersion{
		VersionString: version.String(),
		SemVer:        version,
	}

	sourceCluster.Executor = &testhelper.TestExecutor{}
	source = &utils.Cluster{
		Cluster:    sourceCluster,
		BinDir:     "/source/bindir",
		ConfigPath: filepath.Join(baseDir, utils.SOURCE_CONFIG_FILENAME),
		Version:    gpdbVersion,
	}

	targetCluster.Executor = &testhelper.TestExecutor{}
	target = &utils.Cluster{
		Cluster:    targetCluster,
		BinDir:     "/target/bindir",
		ConfigPath: filepath.Join(baseDir, utils.TARGET_CONFIG_FILENAME),
		Version:    gpdbVersion,
	}

	return
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
