package integrations_test

import (
	"errors"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("upgrade reconfigure ports", func() {
	It("updates status PENDING to COMPLETE if successful", func() {
		Expect(cm.IsPending(upgradestatus.RECONFIGURE_PORTS)).To(BeTrue())

		upgradeReconfigurePortsSession := runCommand("upgrade", "reconfigure-ports")
		Eventually(upgradeReconfigurePortsSession).Should(Exit(0))

		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("sed"))

		Expect(cm.IsComplete(upgradestatus.RECONFIGURE_PORTS)).To(BeTrue())

	})

	It("updates status to FAILED if it fails to run", func() {

		Expect(cm.IsPending(upgradestatus.RECONFIGURE_PORTS)).To(BeTrue())
		testExecutor.LocalError = errors.New("fake test error, reconfigure-ports failed")

		upgradeShareOidsSession := runCommand("upgrade", "reconfigure-ports")
		Eventually(upgradeShareOidsSession).Should(Exit(1))
		Expect(cm.IsFailed(upgradestatus.RECONFIGURE_PORTS)).To(BeTrue())
	})
})
