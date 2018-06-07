package services_test

import (
	"errors"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gpupgrade/testutils"

	"google.golang.org/grpc"

	pb "github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gpupgrade/hub/services"

	"github.com/greenplum-db/gpupgrade/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("hub", func() {
	var (
		hub                *services.Hub
		agentA             *testutils.MockAgentServer
		stubRemoteExecutor *testutils.StubRemoteExecutor
		clusterPair        *services.ClusterPair
	)

	BeforeEach(func() {
		var port int
		agentA, port = testutils.NewMockAgentServer()

		clusterPair = &services.ClusterPair{
			OldCluster: &cluster.Cluster{
				Segments: map[int]cluster.SegConfig{
					0: {DbID: 2, ContentID: 0, Hostname: "localhost", DataDir: "/first/data/dir"},
					1: {DbID: 3, ContentID: 1, Hostname: "localhost", DataDir: "/second/data/dir"},
				},
			},
		}
		conf := &services.HubConfig{
			HubToAgentPort: port,
		}
		stubRemoteExecutor = testutils.NewStubRemoteExecutor()

		hub = services.NewHub(clusterPair, grpc.DialContext, nil, conf, stubRemoteExecutor)
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		agentA.Stop()
	})

	It("receives conversion statuses from the agent and returns all as single message", func() {
		statusMessages := []string{"status", "status"}
		agentA.StatusConversionResponse = &pb.CheckConversionStatusReply{
			Statuses: statusMessages,
		}

		status, err := hub.StatusConversion(nil, &pb.StatusConversionRequest{})
		Expect(err).ToNot(HaveOccurred())

		Expect(status.GetConversionStatuses()).To(Equal([]string{"status", "status"}))
		Expect(agentA.StatusConversionRequest.GetHostname()).To(Equal("localhost"))
		Expect(agentA.StatusConversionRequest.GetSegments()).To(ConsistOf([]*pb.SegmentInfo{
			{
				Content: 0,
				Dbid:    2,
				DataDir: "/first/data/dir",
			},
			{
				Content: 1,
				Dbid:    3,
				DataDir: "/second/data/dir",
			},
		}))
	})

	It("returns an error when AgentConns returns an error", func() {
		agentA.Stop()

		_, err := hub.StatusConversion(nil, &pb.StatusConversionRequest{})
		Expect(err).To(HaveOccurred())
	})

	It("returns an error when Agent server returns an error", func() {
		agentA.Err <- errors.New("any error")

		_, err := hub.StatusConversion(nil, &pb.StatusConversionRequest{})
		Expect(err).To(HaveOccurred())
	})
})
