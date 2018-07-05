package integrations_test

import (
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	"github.com/pkg/errors"
)

var _ = Describe("upgrade validate-start-cluster", func() {
	It("updates status PENDING to RUNNING then to COMPLETE if successful", func(done Done) {
		defer close(done)
		Expect(cm.IsPending(upgradestatus.VALIDATE_START_CLUSTER)).To(BeTrue())

		session := runCommand("upgrade", "validate-start-cluster")
		Eventually(session).Should(Exit(0))

		Expect(testExecutor.NumExecutions).To(Equal(1))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("gpstart"))
		Expect(cm.IsComplete(upgradestatus.VALIDATE_START_CLUSTER)).To(BeTrue())

	})

	It("updates status to FAILED if it fails to run", func() {
		Expect(cm.IsPending(upgradestatus.VALIDATE_START_CLUSTER)).To(BeTrue())

		testExecutor.LocalError = errors.New("start failed")

		session := runCommand("upgrade", "validate-start-cluster")
		Eventually(session).Should(Exit(0))

		Expect(testExecutor.NumExecutions).To(Equal(1))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("gpstart"))
		Expect(cm.IsFailed(upgradestatus.VALIDATE_START_CLUSTER)).To(BeTrue())
	})
})
