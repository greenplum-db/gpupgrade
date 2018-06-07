package services_test

import (
	pb "github.com/greenplum-db/gpupgrade/idl"
	mockpb "github.com/greenplum-db/gpupgrade/mock_idl"

	"github.com/greenplum-db/gpupgrade/hub/services"

	"github.com/golang/mock/gomock"

	"github.com/greenplum-db/gpupgrade/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("object count tests", func() {
	var (
		client *mockpb.MockAgentClient
		ctrl   *gomock.Controller
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		client = mockpb.NewMockAgentClient(ctrl)
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		ctrl.Finish()
	})

	Describe("GetDiskUsageFromSegmentHosts", func() {
		It("returns err msg when unable to call CheckDiskSpaceOnAgents on segment host", func() {
			var clients []services.ClientAndHostname

			client.EXPECT().CheckDiskSpaceOnAgents(
				gomock.Any(),
				&pb.CheckDiskSpaceRequestToAgent{},
			).Return(&pb.CheckDiskSpaceReplyFromAgent{}, errors.New("couldn't connect to hub"))
			clients = append(clients, services.ClientAndHostname{Client: client, Hostname: "doesnotexist"})

			messages := services.GetDiskSpaceFromSegmentHosts(clients)
			Expect(len(messages)).To(Equal(1))
			Expect(messages[0]).To(ContainSubstring("Could not get disk usage from: "))
		})

		It("lists filesystems above usage threshold", func() {
			var clients []services.ClientAndHostname

			var expectedFilesystemsUsage []*pb.FileSysUsage
			expectedFilesystemsUsage = append(expectedFilesystemsUsage, &pb.FileSysUsage{Filesystem: "first filesystem", Usage: 90.4})
			expectedFilesystemsUsage = append(expectedFilesystemsUsage, &pb.FileSysUsage{Filesystem: "/second/filesystem", Usage: 24.2})

			client.EXPECT().CheckDiskSpaceOnAgents(
				gomock.Any(),
				&pb.CheckDiskSpaceRequestToAgent{},
			).Return(&pb.CheckDiskSpaceReplyFromAgent{ListOfFileSysUsage: expectedFilesystemsUsage}, nil)
			clients = append(clients, services.ClientAndHostname{Client: client, Hostname: "doesnotexist"})

			messages := services.GetDiskSpaceFromSegmentHosts(clients)
			Expect(len(messages)).To(Equal(1))
			Expect(messages[0]).To(ContainSubstring("diskspace check - doesnotexist - WARNING first filesystem 90.4 use"))
		})

		It("lists hosts for which all filesystems are below usage threshold", func() {
			var clients []services.ClientAndHostname

			var expectedFilesystemsUsage []*pb.FileSysUsage
			expectedFilesystemsUsage = append(expectedFilesystemsUsage, &pb.FileSysUsage{Filesystem: "first filesystem", Usage: 70.4})
			expectedFilesystemsUsage = append(expectedFilesystemsUsage, &pb.FileSysUsage{Filesystem: "/second/filesystem", Usage: 24.2})

			client.EXPECT().CheckDiskSpaceOnAgents(
				gomock.Any(),
				&pb.CheckDiskSpaceRequestToAgent{},
			).Return(&pb.CheckDiskSpaceReplyFromAgent{ListOfFileSysUsage: expectedFilesystemsUsage}, nil)
			clients = append(clients, services.ClientAndHostname{Client: client, Hostname: "doesnotexist"})

			messages := services.GetDiskSpaceFromSegmentHosts(clients)
			Expect(len(messages)).To(Equal(1))
			Expect(messages[0]).To(ContainSubstring("diskspace check - doesnotexist - OK"))
		})
	})
})
