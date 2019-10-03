package services_test

import (
	"errors"
	"fmt"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"

	"github.com/greenplum-db/gpupgrade/idl/mock_idl"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("upgrade validate start cluster", func() {
	var (
		testExecutor *testhelper.TestExecutor

		ctrl       *gomock.Controller
		mockStream *mock_idl.MockCliToHub_ExecuteServer
	)

	BeforeEach(func() {
		testExecutor = &testhelper.TestExecutor{}
		target.Executor = testExecutor

		ctrl = gomock.NewController(GinkgoT())
		mockStream = mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	It("sets status to COMPLETE when validate start cluster request has been made and returns no error", func() {
		mockStream.EXPECT().
			Send(gomock.Any()).
			AnyTimes()

		Expect(cm.IsPending(upgradestatus.VALIDATE_START_CLUSTER)).To(BeTrue())

		err := hub.ExecuteStartTargetClusterSubStep(mockStream)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() bool { return cm.IsComplete(upgradestatus.VALIDATE_START_CLUSTER) }).Should(BeTrue())
		Expect(testExecutor.NumExecutions).To(Equal(1))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("source /target/bindir/../greenplum_path.sh"))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring(fmt.Sprintf("/target/bindir/gpstart -a -d %s/seg-1", dir)))
	})

	It("sets status to FAILED when the validate start cluster request returns an error", func() {
		mockStream.EXPECT().
			Send(gomock.Any()).
			AnyTimes()

		testExecutor.LocalError = errors.New("some error")

		err := hub.ExecuteStartTargetClusterSubStep(mockStream)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() bool { return cm.IsFailed(upgradestatus.VALIDATE_START_CLUSTER) }).Should(BeTrue())
	})
})
