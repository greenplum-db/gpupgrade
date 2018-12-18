package services_test

import (
	"errors"

	"github.com/golang/mock/gomock"

	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gpupgrade/hub/services"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
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
				agentSegments := []*idl.SegmentInfo{
					{
						Content: int32(segment.ContentID),
						Dbid:    int32(segment.DbID),
						DataDir: segment.DataDir,
					},
				}

				client.EXPECT().CheckConversionStatus(
					gomock.Any(),
					&idl.CheckConversionStatusRequest{
						Segments: agentSegments,
						Hostname: segment.Hostname,
					},
				).Return(
					&idl.CheckConversionStatusReply{Statuses: []*idl.PrimaryStatus{{Status: idl.StepStatus_COMPLETE}}},
					nil,
				).Times(1)
			}

			statuses, err := services.GetConversionStatusFromPrimaries(agentConnections, target)
			Expect(err).ToNot(HaveOccurred())
			Expect(statuses).To(Equal([]*idl.PrimaryStatus{{Status: idl.StepStatus_COMPLETE}, {Status: idl.StepStatus_COMPLETE}}))
		})

		It("returns an error when Agent server returns an error", func() {
			statusMessages := []*idl.PrimaryStatus{{Status: idl.StepStatus_COMPLETE}, {Status: idl.StepStatus_COMPLETE}}
			client.EXPECT().CheckConversionStatus(
				gomock.Any(), // Context
				gomock.Any(), // &pb.CheckConversionStatusRequest
			).Return(
				&idl.CheckConversionStatusReply{Statuses: statusMessages},
				errors.New("agent err"),
			)

			_, err := services.GetConversionStatusFromPrimaries(agentConnections, target)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("PrimaryConversionStatus", func() {

		DescribeTable("PrimaryConversionStatus", func(statuses []*idl.PrimaryStatus, expected idl.StepStatus) {
			mockAgent.StatusConversionResponse = &idl.CheckConversionStatusReply{
				Statuses: statuses,
			}
			status := services.PrimaryConversionStatus(hub)
			Expect(status).To(Equal(expected))
		},
			Entry("returns FAILED if any agents report failure",
				[]*idl.PrimaryStatus{{Status: idl.StepStatus_FAILED}, {Status: idl.StepStatus_RUNNING}},
				idl.StepStatus_FAILED),
			Entry("returns RUNNING if any agents report progress",
				[]*idl.PrimaryStatus{{Status: idl.StepStatus_COMPLETE}, {Status: idl.StepStatus_RUNNING}, {Status: idl.StepStatus_RUNNING}},
				idl.StepStatus_RUNNING),
			Entry("returns COMPLETE if all agents report completion",
				[]*idl.PrimaryStatus{{Status: idl.StepStatus_COMPLETE}, {Status: idl.StepStatus_COMPLETE}},
				idl.StepStatus_COMPLETE),
			Entry("returns PENDING if no agents report any other state",
				[]*idl.PrimaryStatus{{Status: idl.StepStatus_PENDING}, {Status: idl.StepStatus_PENDING}},
				idl.StepStatus_PENDING),
			Entry("returns PENDING if PENDING,PENDING,COMPLETE",
				[]*idl.PrimaryStatus{{Status: idl.StepStatus_PENDING}, {Status: idl.StepStatus_PENDING}, {Status: idl.StepStatus_COMPLETE}},
				idl.StepStatus_PENDING),
			Entry("returns RUNNING if PENDING,RUNNING,COMPLETE",
				[]*idl.PrimaryStatus{{Status: idl.StepStatus_PENDING}, {Status: idl.StepStatus_RUNNING}, {Status: idl.StepStatus_COMPLETE}},
				idl.StepStatus_RUNNING),
		)

		It("returns PENDING if the status is not retrievable", func() {
			mockAgent.Err <- errors.New("any error")
			status := services.PrimaryConversionStatus(hub)
			Expect(status).To(Equal(idl.StepStatus_PENDING))
		})

	})

})
