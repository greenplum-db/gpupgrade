package integrations_test

import (
	"fmt"
	"os"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("check seginstall", func() {
	// `gpupgrade check seginstall` verifies that the user has installed the software on all hosts
	// As a single-node check, this test verifies the mechanics of the check, but would typically succeed.
	// The implementation, however, uses the gpupgrade_agent binary to verify installation. In real life,
	// all the binaries, gpupgrade_hub and gpupgrade_agent included, would be alongside each other.
	// But in our integration tests' context, only the necessary Golang code is compiled, and Ginkgo's default
	// is to compile gpupgrade_hub and gpupgrade_agent in separate directories. As such, this test depends on the
	// setup in `integrations_suite_test.go` to replicate the real-world scenario of "install binaries side-by-side".
	//
	// TODO: This test might be interesting to run multi-node; for that, figure out how "installation" should be done
	It("updates status PENDING to RUNNING then to COMPLETE if successful", func() {
		Expect(cm.IsPending(upgradestatus.SEGINSTALL)).To(BeTrue())

		checkSeginstallSession := runCommand("check", "seginstall", "--master-host", "localhost")
		Eventually(checkSeginstallSession).Should(Exit(0))

		// These assertions are identical to the ones in the hub_check_seginstall unit tests but just to be safe we are leaving it in.
		Expect(testExecutor.NumExecutions).To(Equal(1))

		lsCmd := fmt.Sprintf("ls %s/bin/gpupgrade_agent", os.Getenv("GPHOME"))
		clusterCommands := testExecutor.ClusterCommands[0]
		for _, command := range clusterCommands {
			Expect(command).To(ContainElement(lsCmd))
		}

		Expect(cm.IsComplete(upgradestatus.SEGINSTALL)).To(BeTrue())
	})
})
