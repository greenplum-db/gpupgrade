package integrations_test

import (
	"os"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/onsi/gomega/gbytes"

	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("status", func() {
	BeforeEach(func() {
		go agent.Start()
	})
	Describe("conversion", func() {
		It("Displays status information for all segments", func() {
			pathToSegUpgrade := utils.SegmentPGUpgradeDirectory(testStateDir, 0)
			err := os.MkdirAll(pathToSegUpgrade, 0700)
			Expect(err).ToNot(HaveOccurred())

			f, err := os.Create(filepath.Join(pathToSegUpgrade, "1.done"))
			Expect(err).ToNot(HaveOccurred())
			f.WriteString("Upgrade complete\n")
			f.Close()

			statusSession := runCommand("status", "conversion")
			Eventually(statusSession).Should(Exit(0))

			Eventually(statusSession).Should(gbytes.Say("COMPLETE - DBID 2 - CONTENT ID 0 - PRIMARY - .+"))
		})
	})

	Describe("upgrade", func() {
		It("Reports status from the hub Checklist", func() {
			cm.AddStep(upgradestatus.CONFIG, pb.UpgradeSteps_CONFIG)
			cm.AddStep(upgradestatus.SEGINSTALL, pb.UpgradeSteps_SEGINSTALL)

			statusSession := runCommand("status", "upgrade")
			Eventually(statusSession).Should(Exit(0))

			Eventually(statusSession).Should(gbytes.Say("PENDING - Configuration Check"))
			Eventually(statusSession).Should(gbytes.Say("PENDING - Install binaries on segments"))
		})
	})
})
