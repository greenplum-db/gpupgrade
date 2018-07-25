package services_test

import (
	"errors"
	"io/ioutil"

	"github.com/greenplum-db/gpupgrade/hub/services"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"google.golang.org/grpc"

	"github.com/greenplum-db/gpupgrade/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("hub.UpgradeConvertPrimaries()", func() {
	var (
		dir        string
		hub        *services.Hub
		mockAgent  *testutils.MockAgentServer
		port       int
		oldCluster *cluster.Cluster
		newCluster *cluster.Cluster
		source     *utils.Cluster
		target     *utils.Cluster
		cm         *testutils.MockChecklistManager
	)

	BeforeEach(func() {
		testhelper.SetupTestLogger()

		mockAgent, port = testutils.NewMockAgentServer()

		var err error
		dir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
		conf := &services.HubConfig{
			StateDir:       dir,
			HubToAgentPort: port,
		}

		source, target = testutils.CreateMultinodeSampleClusterPair("/tmp")
		cm = testutils.NewMockChecklistManager()
		hub = services.NewHub(source, target, grpc.DialContext, conf, cm)
	})
	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		defer mockAgent.Stop()
	})

	It("returns nil error, and agent receives only expected segmentConfig values", func() {
		_, err := hub.UpgradeConvertPrimaries(nil,
			&pb.UpgradeConvertPrimariesRequest{})
		Expect(err).ToNot(HaveOccurred())

		Expect(mockAgent.UpgradeConvertPrimarySegmentsRequest.OldBinDir).To(Equal("/old/bin"))
		Expect(mockAgent.UpgradeConvertPrimarySegmentsRequest.NewBinDir).To(Equal("/new/bin"))
		Expect(mockAgent.UpgradeConvertPrimarySegmentsRequest.DataDirPairs).To(ConsistOf([]*pb.DataDirPair{
			{OldDataDir: "old/datadir1", NewDataDir: "new/datadir1", Content: 0, OldPort: 1, NewPort: 11},
			{OldDataDir: "old/datadir2", NewDataDir: "new/datadir2", Content: 1, OldPort: 2, NewPort: 22},
		}))
	})

	It("returns an error if new config does not contain all the same content as the old config", func() {
		clusterPair.NewCluster = &cluster.Cluster{
			ContentIDs: []int{0},
			Segments: map[int]cluster.SegConfig{
				0: newSegment(0, "localhost", "new/datadir1", 11),
			},
		}

		_, err := hub.UpgradeConvertPrimaries(nil,
			&pb.UpgradeConvertPrimariesRequest{})
		Expect(err).To(HaveOccurred())
		Expect(mockAgent.NumberOfCalls()).To(Equal(0))
	})

	It("returns an error if the content matches, but the hostname does not", func() {
		differentSeg := clusterPair.NewCluster.Segments[0]
		differentSeg.Hostname = "localhost2"
		clusterPair.NewCluster.Segments[0] = differentSeg

		_, err := hub.UpgradeConvertPrimaries(nil,
			&pb.UpgradeConvertPrimariesRequest{})
		Expect(err).To(HaveOccurred())

		Expect(mockAgent.NumberOfCalls()).To(Equal(0))
	})

	It("returns an error if any upgrade primary call to any agent fails", func() {
		mockAgent.Err <- errors.New("fail upgrade primary call")

		_, err := hub.UpgradeConvertPrimaries(nil,
			&pb.UpgradeConvertPrimariesRequest{})
		Expect(err).To(HaveOccurred())

		Expect(mockAgent.NumberOfCalls()).To(Equal(1))
	})
})

func newSegment(content int, hostname, dataDir string, port int) cluster.SegConfig {
	return cluster.SegConfig{
		ContentID: content,
		Hostname:  hostname,
		DataDir:   dataDir,
		Port:      port,
	}
}
