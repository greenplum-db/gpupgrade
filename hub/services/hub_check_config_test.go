package services_test

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"gopkg.in/DATA-DOG/go-sqlmock.v1"
)

var _ = Describe("Hub check config", func() {
	var (
		dbConnector *dbconn.DBConn
		mock        sqlmock.Sqlmock
		dir         string
		err         error
		oldBinDir   string
		queryResult = `{"SegConfigs":[{"DbID":1,"ContentID":-1,"Port":15432,"Hostname":"mdw","DataDir":"/data/master/gpseg-1"},` +
			`{"DbID":2,"ContentID":0,"Port":25432,"Hostname":"sdw1","DataDir":"/data/primary/gpseg0"}],"BinDir":"/tmp"}`
		clusterPair *services.ClusterPair
	)

	var expectedClusterPair *services.ClusterPair
	BeforeEach(func() {
		oldBinDir = "/tmp"
		dbConnector, mock = testhelper.CreateAndConnectMockDB(1)
		dir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
		utils.System = utils.InitializeSystemFunctions()
		clusterPair = &services.ClusterPair{}
		expectedClusterPair = &services.ClusterPair{
			OldCluster: &cluster.Cluster{
				ContentIDs: []int{-1, 0},
				Segments: map[int]cluster.SegConfig{
					-1: {DbID: 1, ContentID: -1, Port: 15432, Hostname: "mdw", DataDir: "/data/master/gpseg-1"},
					0:  {DbID: 2, ContentID: 0, Port: 25432, Hostname: "sdw1", DataDir: "/data/primary/gpseg0"},
				},
				Executor: &cluster.GPDBExecutor{},
			},
			OldBinDir: oldBinDir,
		}
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
	})

	It("successfully writes config for GPDB 6", func() {
		testhelper.SetDBVersion(dbConnector, "6.0.0")

		mock.ExpectQuery("SELECT .*").WillReturnRows(getFakeConfigRows())

		fakeConfigFile := gbytes.NewBuffer()

		utils.System.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			fmt.Fprint(fakeConfigFile, string(data))
			ioutil.WriteFile(filename, data, perm)
			return nil
		}

		err := services.SaveOldClusterConfig(clusterPair, dbConnector, dir, oldBinDir)
		Expect(err).ToNot(HaveOccurred())

		Expect(string(fakeConfigFile.Contents())).To(ContainSubstring(queryResult))
		Expect(clusterPair).To(Equal(expectedClusterPair))
	})

	// The database is running, master-host is provided, and connection is successful
	// writes the resulting rows according to however the provided writer does it
	It("successfully writes config for GPDB 4 and 5", func() {
		mock.ExpectQuery("SELECT .*").WillReturnRows(getFakeConfigRows())

		fakeConfigFile := gbytes.NewBuffer()

		utils.System.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			fmt.Fprint(fakeConfigFile, string(data))
			ioutil.WriteFile(filename, data, perm)
			return nil
		}

		err := services.SaveOldClusterConfig(clusterPair, dbConnector, dir, oldBinDir)
		Expect(err).ToNot(HaveOccurred())

		Expect(string(fakeConfigFile.Contents())).To(ContainSubstring(queryResult))
		Expect(clusterPair).To(Equal(expectedClusterPair))
	})

	It("fails to write file", func() {
		utils.System.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return errors.New("failed to write config file")
		}

		err := services.SaveOldClusterConfig(clusterPair, dbConnector, dir, oldBinDir)
		Expect(err).To(HaveOccurred())
	})

	It("db.Select query for cluster config fails", func() {
		mock.ExpectQuery("SELECT .*").WillReturnError(errors.New("fail config query"))

		utils.System.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return nil
		}

		err := services.SaveOldClusterConfig(clusterPair, dbConnector, dir, oldBinDir)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("Unable to get segment configuration for old cluster: fail config query"))
	})
})

// Construct sqlmock in-memory rows that are structured properly
func getFakeConfigRows() *sqlmock.Rows {
	header := []string{"dbid", "contentid", "port", "hostname", "datadir"}
	fakeConfigRow := []driver.Value{1, -1, 15432, "mdw", "/data/master/gpseg-1"}
	fakeConfigRow2 := []driver.Value{2, 0, 25432, "sdw1", "/data/primary/gpseg0"}
	rows := sqlmock.NewRows(header)
	heapfakeResult := rows.AddRow(fakeConfigRow...).AddRow(fakeConfigRow2...)
	return heapfakeResult
}
