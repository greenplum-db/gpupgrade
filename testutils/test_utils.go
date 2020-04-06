package testutils

import (
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"

	"github.com/greenplum-db/gpupgrade/greenplum"
)

var UpgradeID = upgrade.ID(10) // "CgAAAAAAAAA" in base64

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

func SetupDataDirs(t *testing.T) (source, target, tmpDir string) {

	var err error
	tmpDir, err = ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	source = createDataDir(t, "source", tmpDir)
	target = createDataDir(t, "target", tmpDir)

	return source, target, tmpDir
}

func createDataDir(t *testing.T, name, tmpDir string) (source string) {

	source = filepath.Join(tmpDir, name)

	err := os.Mkdir(source, 0700)
	if err != nil {
		t.Errorf("unexpected err: %v", err)
	}

	for _, fileName := range utils.PostgresFiles {
		filePath := filepath.Join(source, fileName)
		err = os.Mkdir(filePath, 0700)
		if err != nil {
			t.Errorf("unexpected err: %v", err)
		}
	}

	return source
}
