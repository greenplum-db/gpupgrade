package services_test

import (
	"errors"
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("upgrade validate start cluster", func() {
	var (
		testExecutor *testhelper.TestExecutor
	)

	BeforeEach(func() {
		testExecutor = &testhelper.TestExecutor{}
		target.Executor = testExecutor
	})

	It("sets status to COMPLETE when validate start cluster request has been made and returns no error", func() {
		Expect(cm.IsPending(upgradestatus.VALIDATE_START_CLUSTER)).To(BeTrue())

		_, err := hub.UpgradeValidateStartCluster(nil, &pb.UpgradeValidateStartClusterRequest{})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() bool { return cm.IsComplete(upgradestatus.VALIDATE_START_CLUSTER) }).Should(BeTrue())
		Expect(testExecutor.NumExecutions).To(Equal(1))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("source /target/bindir/../greenplum_path.sh"))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring(fmt.Sprintf("/target/bindir/gpstart -a -d %s/seg-1", dir)))
	})

	It("sets status to FAILED when the validate start cluster request returns an error", func() {
		testExecutor.LocalError = errors.New("some error")

		_, err := hub.UpgradeValidateStartCluster(nil, &pb.UpgradeValidateStartClusterRequest{})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() bool { return cm.IsFailed(upgradestatus.VALIDATE_START_CLUSTER) }).Should(BeTrue())
	})
})
