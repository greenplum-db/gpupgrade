package utils_test

import (
	"database/sql/driver"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/operating"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	sqlmock "gopkg.in/DATA-DOG/go-sqlmock.v1"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cluster", func() {
	var (
		expectedCluster *utils.Cluster
		testStateDir    string
		err             error
	)

	BeforeEach(func() {
		testStateDir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		testhelper.SetupTestLogger()
		expectedCluster = &utils.Cluster{
			Cluster:    testutils.CreateMultinodeSampleCluster("/tmp"),
			BinDir:     "/fake/path",
			ConfigPath: path.Join(testStateDir, "cluster_config.json"),
		}
	})

	AfterEach(func() {
		os.RemoveAll(testStateDir)
		utils.System = utils.InitializeSystemFunctions()
	})

	Describe("Commit and Load", func() {
		It("can save a config and successfully load it back in", func() {
			err := expectedCluster.Commit()
			Expect(err).ToNot(HaveOccurred())
			givenCluster := &utils.Cluster{
				ConfigPath: path.Join(testStateDir, "cluster_config.json"),
			}
			err = givenCluster.Load()
			Expect(err).ToNot(HaveOccurred())

			// We don't serialize the Executor
			givenCluster.Executor = expectedCluster.Executor

			Expect(expectedCluster).To(Equal(givenCluster))
		})
	})

	Describe("PrimaryHostnames", func() {
		It("returns a list of hosts for only the primaries", func() {
			hostnames := expectedCluster.PrimaryHostnames()
			Expect(hostnames).To(ConsistOf([]string{"host1", "host2"}))
		})
	})

	Describe("SegmentsOn", func() {
		It("returns an error for an unknown hostname", func() {
			c := utils.Cluster{Cluster: &cluster.Cluster{}}
			_, err := c.SegmentsOn("notahost")
			Expect(err).To(HaveOccurred())
		})

		It("maps all hosts to segment configurations", func() {
			expected := map[string][]cluster.SegConfig{
				"localhost": {expectedCluster.Segments[-1]},
				"host1":     {expectedCluster.Segments[0]},
				"host2":     {expectedCluster.Segments[1]},
			}
			for host, expectedSegments := range expected {
				segments, err := expectedCluster.SegmentsOn(host)
				Expect(err).NotTo(HaveOccurred())
				Expect(segments).To(ConsistOf(expectedSegments))
			}
		})

		It("groups all segments by hostname", func() {
			c := utils.Cluster{
				Cluster: &cluster.Cluster{
					ContentIDs: []int{-1, 0, 1},
					Segments: map[int]cluster.SegConfig{
						-1: {ContentID: -1, DbID: 1, Port: 15432, Hostname: "mdw", DataDir: "/seg-1"},
						0:  {ContentID: 0, DbID: 2, Port: 25432, Hostname: "sdw1", DataDir: "/seg1"},
						1:  {ContentID: 1, DbID: 3, Port: 25433, Hostname: "sdw1", DataDir: "/seg2"},
					},
				},
			}

			expected := map[string][]cluster.SegConfig{
				"mdw":  {c.Segments[-1]},
				"sdw1": {c.Segments[0], c.Segments[1]},
			}
			for host, expectedSegments := range expected {
				segments, err := c.SegmentsOn(host)
				Expect(err).NotTo(HaveOccurred())
				Expect(segments).To(ConsistOf(expectedSegments))
			}
		})
	})

	Describe("ExecuteOnAllHosts", func() {
		It("returns an error for an unloaded cluster", func() {
			emptyCluster := &utils.Cluster{Cluster: &cluster.Cluster{}}

			_, err := emptyCluster.ExecuteOnAllHosts("description", func(int) string { return "" })
			Expect(err).To(HaveOccurred())
		})

		It("executes a command on each separate host", func() {
			executor := &testhelper.TestExecutor{}
			expectedCluster.Executor = executor

			_, err := expectedCluster.ExecuteOnAllHosts("description",
				func(contentID int) string {
					return fmt.Sprintf("command %d", contentID)
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(len(executor.ClusterCommands)).To(Equal(1))
			for _, id := range expectedCluster.ContentIDs {
				Expect(executor.ClusterCommands[0][id]).To(ContainElement(fmt.Sprintf("command %d", id)))
			}
		})
	})

	Describe("NewDBConn", func() {
		var (
			originalEnv string
			c           *utils.Cluster
		)

		BeforeEach(func() {
			master := cluster.SegConfig{
				DbID:      1,
				ContentID: -1,
				Port:      5432,
				Hostname:  "mdw",
			}

			originalEnv = os.Getenv("PGUSER")

			cc := cluster.Cluster{Segments: map[int]cluster.SegConfig{-1: master}}
			c = &utils.Cluster{Cluster: &cc}

		})

		AfterEach(func() {
			os.Setenv("PGUSER", originalEnv)
			operating.System = operating.InitializeSystemFunctions()
		})

		It("can construct a dbconn from a cluster", func() {
			expectedUser := "brother_maynard"
			os.Setenv("PGUSER", expectedUser)

			dbConnector := c.NewDBConn()

			Expect(dbConnector.DBName).To(Equal("postgres"))
			Expect(dbConnector.Host).To(Equal("mdw"))
			Expect(dbConnector.Port).To(Equal(5432))
			Expect(dbConnector.User).To(Equal(expectedUser))
		})

		It("sets the default user if neither PGUSER or current user are available", func() {
			os.Unsetenv("PGUSER")

			operating.System.CurrentUser = func() (*user.User, error) {
				return nil, errors.New("Your systems is seriously borked if this fails")
			}

			dbConnector := c.NewDBConn()

			Expect(dbConnector.User).To(Equal("gpadmin"))
		})
		// FIXME: protect against badly initialized clusters
	})
	Describe("RefreshConfig", func() {
		var (
			expectedCluster *utils.Cluster
			resultCluster   *utils.Cluster
			dbConnector     *dbconn.DBConn
			mockdb          *sqlx.DB
			mock            sqlmock.Sqlmock
		)

		BeforeEach(func() {
			expectedCluster = &utils.Cluster{
				Cluster: &cluster.Cluster{
					ContentIDs: []int{-1, 0},
					Segments: map[int]cluster.SegConfig{
						-1: {DbID: 1, ContentID: -1, Port: 15432, Hostname: "mdw", DataDir: "/data/master/gpseg-1"},
						0:  {DbID: 2, ContentID: 0, Port: 25432, Hostname: "sdw1", DataDir: "/data/primary/gpseg0"},
					},
					Executor: &cluster.GPDBExecutor{},
				},
			}
			resultCluster = utils.NewMasterOnlyCluster(5432, "mdw", "", "")

			mockdb, mock = testhelper.CreateMockDB()
			testDriver := testhelper.TestDriver{DB: mockdb, DBName: "testdb", User: "testrole"}
			dbConnector = dbconn.NewDBConn(testDriver.DBName, testDriver.User, "fakehost", -1 /* not used */)
			dbConnector.Driver = testDriver
		})

		getFakeVersionRow := func(v string) *sqlmock.Rows {
			return sqlmock.NewRows([]string{"versionstring"}).
				AddRow([]driver.Value{"PostgreSQL 8.4 (Greenplum Database " + v + ")"}...)
		}

		// Construct sqlmock in-memory rows that are structured properly
		getFakeConfigRows := func() *sqlmock.Rows {
			header := []string{"dbid", "contentid", "port", "hostname", "datadir"}
			fakeConfigRow := []driver.Value{1, -1, 15432, "mdw", "/data/master/gpseg-1"}
			fakeConfigRow2 := []driver.Value{2, 0, 25432, "sdw1", "/data/primary/gpseg0"}
			rows := sqlmock.NewRows(header)
			heapfakeResult := rows.AddRow(fakeConfigRow...).AddRow(fakeConfigRow2...)
			return heapfakeResult
		}

		It("successfully stores target cluster config for GPDB 6", func() {
			mock.ExpectQuery("SELECT version()").WillReturnRows(getFakeVersionRow("6.0.0"))
			mock.ExpectQuery("SELECT .*").WillReturnRows(getFakeConfigRows())

			err := resultCluster.RefreshConfig(dbConnector)
			Expect(err).ToNot(HaveOccurred())
			Expect(resultCluster).To(Equal(expectedCluster))
		})

		It("successfully stores target cluster config for GPDB 4 and 5", func() {
			mock.ExpectQuery("SELECT version()").WillReturnRows(getFakeVersionRow("5.10.1"))
			mock.ExpectQuery("SELECT .*").WillReturnRows(getFakeConfigRows())

			err := resultCluster.RefreshConfig(dbConnector)
			Expect(err).ToNot(HaveOccurred())

			Expect(resultCluster).To(Equal(expectedCluster))
		})

		It("db.Select query for cluster config fails", func() {
			mock.ExpectQuery("SELECT version()").WillReturnRows(getFakeVersionRow("5.10.1"))
			mock.ExpectQuery("SELECT .*").WillReturnError(errors.New("fail config query"))

			utils.System.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
				return nil
			}

			err := resultCluster.RefreshConfig(dbConnector)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("Unable to get segment configuration for cluster: fail config query"))
		})
	})
})
