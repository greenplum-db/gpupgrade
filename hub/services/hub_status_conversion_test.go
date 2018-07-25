package services_test

import (
	"errors"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/cluster"
	mockpb "github.com/greenplum-db/gpupgrade/mock_idl"
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
		hub               *services.Hub
		agentA            *testutils.MockAgentServer
		source            *utils.Cluster
		target            *utils.Cluster
		cm                *testutils.MockChecklistManager
		ctrl              *gomock.Controller
		mockAgent1        *mockpb.MockAgentClient
		agentConnections  []*services.Connection
		hostToSegmentsMap map[string][]cluster.SegConfig
	)

	BeforeEach(func() {
		var port int
		agentA, port = testutils.NewMockAgentServer()

		source, target = testutils.CreateMultinodeSampleClusterPair("/tmp")
		conf := &services.HubConfig{
			HubToAgentPort: port,
		}

		cm = testutils.NewMockChecklistManager()
		hub = services.NewHub(source, target, grpc.DialContext, conf, cm)

		ctrl = gomock.NewController(GinkgoT())
		mockAgent1 = mockpb.NewMockAgentClient(ctrl)

		agentConnections = []*services.Connection{
			{nil, mockAgent1, "host1", nil},
		}

		hostToSegmentsMap = make(map[string][]cluster.SegConfig, 0)
		hostToSegmentsMap["host1"] = []cluster.SegConfig{
			newSegment(0, "host1", "old/datadir1", 1),
		}
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		agentA.Stop()
		ctrl.Finish()
	})

	Describe("GetConversionStatusFromPrimaries", func() {
		It("receives conversion statuses from the agent and returns all as single message", func() {
			statusMessages := []string{"status", "status"}
			segment1 := hostToSegmentsMap["host1"][0]
			var agentSegments []*pb.SegmentInfo
			agentSegments = append(
				agentSegments,
				&pb.SegmentInfo{
					Content: int32(segment1.ContentID),
					Dbid:    int32(segment1.DbID),
					DataDir: segment1.DataDir,
				},
			)

			mockAgent1.EXPECT().CheckConversionStatus(
				gomock.Any(),
				&pb.CheckConversionStatusRequest{
					Segments: agentSegments,
					Hostname: segment1.Hostname,
				},
			).Return(
				&pb.CheckConversionStatusReply{Statuses: statusMessages},
				nil,
			).Times(1)

			statuses, err := services.GetConversionStatusFromPrimaries(agentConnections, hostToSegmentsMap)
			Expect(err).ToNot(HaveOccurred())
			Expect(statuses).To(Equal([]string{"status", "status"}))
		})

		It("returns an error when Agent server returns an error", func() {
			statusMessages := []string{"status", "status"}
			mockAgent1.EXPECT().CheckConversionStatus(
				gomock.Any(), // Context
				gomock.Any(), // &pb.CheckConversionStatusRequest
			).Return(
				&pb.CheckConversionStatusReply{Statuses: statusMessages},
				errors.New("agent err"),
			)

			_, err := services.GetConversionStatusFromPrimaries(agentConnections, hostToSegmentsMap)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("PrimaryConversionStatus", func() {
		It("returns FAILED if any agents report failure", func() {
			agentA.StatusConversionResponse = &pb.CheckConversionStatusReply{
				Statuses: []string{"FAILED", "RUNNING"},
			}

			status := services.PrimaryConversionStatus(hub)
			Expect(status).To(Equal(pb.StepStatus_FAILED))
		})

		It("returns RUNNING if any agents report progress", func() {
			agentA.StatusConversionResponse = &pb.CheckConversionStatusReply{
				Statuses: []string{"COMPLETE", "RUNNING"},
			}

			status := services.PrimaryConversionStatus(hub)
			Expect(status).To(Equal(pb.StepStatus_RUNNING))
		})

		It("returns COMPLETE if all agents report completion", func() {
			agentA.StatusConversionResponse = &pb.CheckConversionStatusReply{
				Statuses: []string{"COMPLETE", "COMPLETE"},
			}

			status := services.PrimaryConversionStatus(hub)
			Expect(status).To(Equal(pb.StepStatus_COMPLETE))
		})

		It("returns PENDING if no agents report any other state", func() {
			agentA.StatusConversionResponse = &pb.CheckConversionStatusReply{
				Statuses: []string{"PENDING", "PENDING"},
			}

			status := services.PrimaryConversionStatus(hub)
			Expect(status).To(Equal(pb.StepStatus_PENDING))
		})

		It("returns PENDING if the status is not retrievable", func() {
			agentA.Err <- errors.New("any error")

			status := services.PrimaryConversionStatus(hub)
			Expect(status).To(Equal(pb.StepStatus_PENDING))
		})
	})
})
