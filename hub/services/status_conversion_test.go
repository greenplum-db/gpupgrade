package services_test

import (
	"errors"

	"github.com/golang/mock/gomock"

	pb "github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gpupgrade/hub/services"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("hub", func() {
	var (
		agentConnections []*services.Connection
	)

	BeforeEach(func() {
		agentConnections = []*services.Connection{
			{nil, client, "host1", nil},
			{nil, client, "host2", nil},
		}
	})

	Describe("GetConversionStatusFromPrimaries", func() {
		It("receives conversion statuses from the agent and returns all as single message", func() {
			for id := 0; id <= 1; id++ {
				segment := target.Segments[id]
				agentSegments := []*pb.SegmentInfo{
					{
						Content: int32(segment.ContentID),
						Dbid:    int32(segment.DbID),
						DataDir: segment.DataDir,
					},
				}

				client.EXPECT().CheckConversionStatus(
					gomock.Any(),
					&pb.CheckConversionStatusRequest{
						Segments: agentSegments,
						Hostname: segment.Hostname,
					},
				).Return(
					&pb.CheckConversionStatusReply{Statuses: []*pb.PrimaryStatus{{Status: pb.StepStatus_COMPLETE}}},
					nil,
				).Times(1)
			}

			statuses, err := services.GetConversionStatusFromPrimaries(agentConnections, target)
			Expect(err).ToNot(HaveOccurred())
			Expect(statuses).To(Equal([]*pb.PrimaryStatus{{Status: pb.StepStatus_COMPLETE}, {Status: pb.StepStatus_COMPLETE}}))
		})

		It("returns an error when Agent server returns an error", func() {
			statusMessages := []*pb.PrimaryStatus{{Status: pb.StepStatus_COMPLETE}, {Status: pb.StepStatus_COMPLETE}}
			client.EXPECT().CheckConversionStatus(
				gomock.Any(), // Context
				gomock.Any(), // &pb.CheckConversionStatusRequest
			).Return(
				&pb.CheckConversionStatusReply{Statuses: statusMessages},
				errors.New("agent err"),
			)

			_, err := services.GetConversionStatusFromPrimaries(agentConnections, target)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("PrimaryConversionStatus", func() {
		It("returns FAILED if any agents report failure", func() {
			mockAgent.StatusConversionResponse = &pb.CheckConversionStatusReply{
				Statuses: []*pb.PrimaryStatus{{Status: pb.StepStatus_FAILED}, {Status: pb.StepStatus_RUNNING}},
			}

			status := services.PrimaryConversionStatus(hub)
			Expect(status).To(Equal(pb.StepStatus_FAILED))
		})

		It("returns RUNNING if any agents report progress", func() {
			mockAgent.StatusConversionResponse = &pb.CheckConversionStatusReply{
				Statuses: []*pb.PrimaryStatus{{Status: pb.StepStatus_COMPLETE}, {Status: pb.StepStatus_RUNNING}},
			}

			status := services.PrimaryConversionStatus(hub)
			Expect(status).To(Equal(pb.StepStatus_RUNNING))
		})

		It("returns COMPLETE if all agents report completion", func() {
			mockAgent.StatusConversionResponse = &pb.CheckConversionStatusReply{
				Statuses: []*pb.PrimaryStatus{{Status: pb.StepStatus_COMPLETE}, {Status: pb.StepStatus_COMPLETE}},
			}

			status := services.PrimaryConversionStatus(hub)
			Expect(status).To(Equal(pb.StepStatus_COMPLETE))
		})

		It("returns PENDING if no agents report any other state", func() {
			mockAgent.StatusConversionResponse = &pb.CheckConversionStatusReply{
				Statuses: []*pb.PrimaryStatus{{Status: pb.StepStatus_PENDING}, {Status: pb.StepStatus_PENDING}},
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
