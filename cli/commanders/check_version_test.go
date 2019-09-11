package commanders_test

import (
	"errors"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ bool = Describe("object count tests", func() {

	var (
		client *mock_idl.MockCliToHubClient
		ctrl   *gomock.Controller
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		client = mock_idl.NewMockCliToHubClient(ctrl)
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		defer ctrl.Finish()
	})
	Describe("Execute", func() {
		It("prints out version check is OK and that check version request was processed", func() {
			client.EXPECT().CheckVersion(
				gomock.Any(),
				&idl.CheckVersionRequest{},
			).Return(&idl.CheckVersionReply{IsVersionCompatible: true}, nil)
			request := commanders.NewVersionChecker(client)
			err := request.Execute()
			Expect(err).To(BeNil())
		})
		It("prints out version check failed and that check version request was processed", func() {
			client.EXPECT().CheckVersion(
				gomock.Any(),
				&idl.CheckVersionRequest{},
			).Return(&idl.CheckVersionReply{IsVersionCompatible: false}, nil)
			request := commanders.NewVersionChecker(client)
			err := request.Execute()
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).Should(ContainSubstring("Version Compatibility Check Failed"))
		})
		It("prints out that it was unable to connect to hub", func() {
			client.EXPECT().CheckVersion(
				gomock.Any(),
				&idl.CheckVersionRequest{},
			).Return(&idl.CheckVersionReply{IsVersionCompatible: false}, errors.New("something went wrong"))
			request := commanders.NewVersionChecker(client)
			err := request.Execute()
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).Should(ContainSubstring("gRPC call to hub failed"))
		})
	})
})
