package services_test

import (
	"errors"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/cluster"

	pb "github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gpupgrade/hub/services"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("hub", func() {
	var (
		agentConnections  []*services.Connection
		hostToSegmentsMap map[string][]cluster.SegConfig
	)

	BeforeEach(func() {
		agentConnections = []*services.Connection{
			{nil, client, "host1", nil},
		}

		hostToSegmentsMap = make(map[string][]cluster.SegConfig, 0)
		hostToSegmentsMap["host1"] = []cluster.SegConfig{
			newSegment(0, "host1", "old/datadir1", 1),
		}
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

			client.EXPECT().CheckConversionStatus(
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
			client.EXPECT().CheckConversionStatus(
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
			mockAgent.StatusConversionResponse = &pb.CheckConversionStatusReply{
				Statuses: []string{"FAILED", "RUNNING"},
			}

			status := services.PrimaryConversionStatus(hub)
			Expect(status).To(Equal(pb.StepStatus_FAILED))
		})

		It("returns RUNNING if any agents report progress", func() {
			mockAgent.StatusConversionResponse = &pb.CheckConversionStatusReply{
				Statuses: []string{"COMPLETE", "RUNNING"},
			}

			status := services.PrimaryConversionStatus(hub)
			Expect(status).To(Equal(pb.StepStatus_RUNNING))
		})

		It("returns COMPLETE if all agents report completion", func() {
			mockAgent.StatusConversionResponse = &pb.CheckConversionStatusReply{
				Statuses: []string{"COMPLETE", "COMPLETE"},
			}

			status := services.PrimaryConversionStatus(hub)
			Expect(status).To(Equal(pb.StepStatus_COMPLETE))
		})

		It("returns PENDING if no agents report any other state", func() {
			mockAgent.StatusConversionResponse = &pb.CheckConversionStatusReply{
				Statuses: []string{"PENDING", "PENDING"},
			}

			status := services.PrimaryConversionStatus(hub)
			Expect(status).To(Equal(pb.StepStatus_PENDING))
		})

		It("returns PENDING if the status is not retrievable", func() {
			mockAgent.Err <- errors.New("any error")

			status := services.PrimaryConversionStatus(hub)
			Expect(status).To(Equal(pb.StepStatus_PENDING))
		})
	})
})
