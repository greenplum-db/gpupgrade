package commanders_test

import (
	"errors"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("reporter", func() {
	var (
		client *mock_idl.MockCliToHubClient
		ctrl   *gomock.Controller

		hubClient  *testutils.MockHubClient
		upgrader   *commanders.Upgrader
		testStdout *gbytes.Buffer
		testStderr *gbytes.Buffer
	)

	BeforeEach(func() {
		testStdout, testStderr, _ = testhelper.SetupTestLogger()

		ctrl = gomock.NewController(GinkgoT())
		client = mock_idl.NewMockCliToHubClient(ctrl)

		hubClient = testutils.NewMockHubClient()
		upgrader = commanders.NewUpgrader(hubClient)
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		defer ctrl.Finish()
	})

	Describe("ConvertMaster", func() {
		It("Reports success when pg_upgrade started", func() {
			client.EXPECT().UpgradeConvertMaster(
				gomock.Any(),
				&idl.UpgradeConvertMasterRequest{},
			).Return(&idl.UpgradeConvertMasterReply{}, nil)
			err := commanders.NewUpgrader(client).ConvertMaster()
			Expect(err).To(BeNil())
			Eventually(testStdout).Should(gbytes.Say("Kicked off pg_upgrade request"))
		})

		It("reports failure when command fails to connect to the hub", func() {
			client.EXPECT().UpgradeConvertMaster(
				gomock.Any(),
				&idl.UpgradeConvertMasterRequest{},
			).Return(&idl.UpgradeConvertMasterReply{}, errors.New("something bad happened"))
			err := commanders.NewUpgrader(client).ConvertMaster()
			Expect(err).ToNot(BeNil())
			Eventually(testStderr).Should(gbytes.Say("ERROR - Unable to connect to hub"))

		})
	})

	Describe("ConvertPrimaries", func() {
		It("returns no error when the hub returns no error", func() {
			err := upgrader.ConvertPrimaries()
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error when the hub returns an error", func() {
			hubClient.Err = errors.New("hub error")

			err := upgrader.ConvertPrimaries()
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ShareOids", func() {
		It("returns no error when oids are shared successfully", func() {
			err := upgrader.ShareOids()
			Expect(err).ToNot(HaveOccurred())

			Expect(hubClient.UpgradeShareOidsRequest).To(Equal(&idl.UpgradeShareOidsRequest{}))
		})

		It("returns an error when oids cannot be shared", func() {
			hubClient.Err = errors.New("test share oids failed")

			err := upgrader.ShareOids()
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ReconfigurePorts", func() {
		It("returns nil error when ports are reconfigured successfully", func() {
			err := upgrader.ReconfigurePorts()
			Expect(err).ToNot(HaveOccurred())

			Expect(hubClient.UpgradeReconfigurePortsRequest).To(Equal(&idl.UpgradeReconfigurePortsRequest{}))
		})

		It("returns error when ports cannot be reconfigured", func() {
			hubClient.Err = errors.New("reconfigure ports failed")

			err := upgrader.ReconfigurePorts()
			Expect(err).To(HaveOccurred())
		})
	})
})
