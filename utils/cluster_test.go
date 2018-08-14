package utils_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
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
			segments, err := expectedCluster.SegmentsOn("host1")
			Expect(err).NotTo(HaveOccurred())
			Expect(segments).To(Equal([]cluster.SegConfig{expectedCluster.Segments[0]}))

			segments, err = expectedCluster.SegmentsOn("host2")
			Expect(err).NotTo(HaveOccurred())
			Expect(segments).To(Equal([]cluster.SegConfig{expectedCluster.Segments[1]}))

			segments, err = expectedCluster.SegmentsOn("localhost")
			Expect(err).To(HaveOccurred())
		})

		It("groups all segments by hostname", func() {
			seg1 := expectedCluster.Segments[1]
			seg1.Hostname = "host1"
			expectedCluster.Segments[1] = seg1

			expected := []cluster.SegConfig{expectedCluster.Segments[0], expectedCluster.Segments[1]}
			segments, err := expectedCluster.SegmentsOn("host1")
			Expect(err).NotTo(HaveOccurred())
			Expect(segments).To(ConsistOf(expected))

			segments, err = expectedCluster.SegmentsOn("localhost")
			Expect(err).To(HaveOccurred())
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

	Describe("ClusterFromDB", func() {
		It("returns an error if connection fails", func() {
			connErr := errors.New("connection failed")
			conn := dbconn.NewDBConnFromEnvironment("testdb")
			conn.Driver = testhelper.TestDriver{ErrToReturn: connErr}

			cluster, err := utils.ClusterFromDB(conn, "", "")

			Expect(err).To(HaveOccurred())
			Expect(cluster).To(BeNil())
			Expect(err.Error()).To(ContainSubstring(connErr.Error()))
		})

		It("returns an error if the segment configuration query fails", func() {
			conn, mock := testutils.CreateMockDBConn()
			testutils.SetMockGPDBVersion(mock, "5.3.4")

			queryErr := errors.New("failed to get segment configuration")
			mock.ExpectQuery("SELECT .* FROM gp_segment_configuration").WillReturnError(queryErr)

			cluster, err := utils.ClusterFromDB(conn, "", "")

			Expect(err).To(HaveOccurred())
			Expect(cluster).To(BeNil())
			Expect(err.Error()).To(ContainSubstring(queryErr.Error()))
		})

		It("populates a cluster using DB information", func() {
			conn, mock := testutils.CreateMockDBConn()

			testutils.SetMockGPDBVersion(mock, "5.3.4")
			mock.ExpectQuery("SELECT .* FROM gp_segment_configuration").WillReturnRows(testutils.MockSegmentConfiguration())

			binDir := "/usr/local/gpdb/bin"
			configPath := "/tmp/config.json"

			cluster, err := utils.ClusterFromDB(conn, binDir, configPath)
			Expect(err).NotTo(HaveOccurred())

			Expect(cluster.Cluster).To(Equal(testutils.MockCluster()))
			Expect(cluster.BinDir).To(Equal(binDir))
			Expect(cluster.ConfigPath).To(Equal(configPath))
		})
	})
})
