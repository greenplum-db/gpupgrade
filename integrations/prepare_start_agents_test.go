package integrations_test

import (
	"fmt"
	"os"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("prepare start-agents", func() {
	It("updates status PENDING to RUNNING then to COMPLETE if successful", func() {
		Expect(cm.IsPending(upgradestatus.START_AGENTS)).To(BeTrue())

		prepareStartAgentsSession := runCommand("prepare", "start-agents")
		Eventually(prepareStartAgentsSession).Should(Exit(0))

		// These assertions are identical to the ones in the prepare_start_agent unit tests but just to be safe we are leaving it in.
		Expect(testExecutor.NumExecutions).To(Equal(1))

		startAgentsCmd := fmt.Sprintf("%s/bin/gpupgrade_agent --daemonize", os.Getenv("GPHOME"))
		clusterCommands := testExecutor.ClusterCommands[0]
		for _, command := range clusterCommands {
			Expect(command).To(ContainElement(startAgentsCmd))
		}

		Expect(cm.IsComplete(upgradestatus.START_AGENTS)).To(BeTrue())
	})
})
