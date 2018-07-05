package integrations_test

import (
	"errors"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("upgrade share oids", func() {
	BeforeEach(func() {
		go agent.Start()
	})
	It("updates status PENDING to RUNNING then to COMPLETE if successful", func() {

		Expect(cm.IsPending(upgradestatus.SHARE_OIDS)).To(BeTrue())

		upgradeShareOidsSession := runCommand("upgrade", "share-oids")
		Eventually(upgradeShareOidsSession).Should(Exit(0))

		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("rsync"))
		Expect(cm.IsComplete(upgradestatus.SHARE_OIDS)).To(BeTrue())

	})

	It("updates status to FAILED if it fails to run", func() {

		Expect(cm.IsPending(upgradestatus.SHARE_OIDS)).To(BeTrue())
		testExecutor.LocalError = errors.New("fake test error, share oid failed to send files")

		upgradeShareOidsSession := runCommand("upgrade", "share-oids")
		Eventually(upgradeShareOidsSession).Should(Exit(0))
		Expect(cm.IsFailed(upgradestatus.SHARE_OIDS)).To(BeTrue())
	})
})
