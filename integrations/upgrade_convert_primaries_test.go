package integrations_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("upgrade convert primaries", func() {
	/*
	 * The two pending tests in this block are useful tests in theory, but
	 * would rely on using an actual checklist manager over a mock checklist
	 * manager due to the way status checking is done for pg_upgrade.
	 *
	 * If convert-primaries were modified to somehow use the actual checklist
	 * manager, these tests would be meaningful and can be refactored and
	 * fixed up appropriately.  Otherwise, it would make more sense to
	 * replace these with end-to-end tests, as in that case they would grow
	 * beyond the scope of integration tests due to a lack of mocking.
	 *
	 * TODO: Decide how to handle convert-primaries testing.
	 */
	var (
		oidFile string
	)

	BeforeEach(func() {
		var err error

		err = os.MkdirAll(filepath.Join(testStateDir, "convert-master"), 0700)
		Expect(err).ToNot(HaveOccurred())
		for i := range []int{0, 1, 2} {
			oidFile = filepath.Join(testStateDir, "convert-master", fmt.Sprintf("pg_upgrade_dump_seg%d_oids.sql", i))
			f, err := os.Create(oidFile)
			Expect(err).ToNot(HaveOccurred())
			f.Close()
		}

		go agent.Start()
	})

	XIt("updates status PENDING to RUNNING then to COMPLETE if successful", func() {
		utils.System.RunCommandAsync = func(cmdStr string, logFile string) error {
			_, err := agentExecutor.ExecuteLocalCommand(cmdStr)
			return err
		}

		cm.AddStep(upgradestatus.CONVERT_PRIMARIES, idl.UpgradeSteps_CONVERT_PRIMARIES)

		Expect(runStatusUpgrade()).To(ContainSubstring("PENDING - Run pg_upgrade on primaries"))
		testExecutor.LocalOutput = "TEST"

		step := cm.GetStepWriter(upgradestatus.START_AGENTS)
		step.MarkInProgress()
		step.MarkComplete()

		agentExecutor.LocalOutput = "run pg_upgrade for segment"

		upgradeConvertPrimaries := runCommand(
			"upgrade",
			"convert-primaries",
		)
		Expect(upgradeConvertPrimaries).To(Exit(0))

		step = cm.GetStepWriter(upgradestatus.CONVERT_PRIMARIES)
		step.MarkInProgress()

		agentExecutor.LocalOutput = "pgrep for running pg_upgrade for segment"
		Expect(runStatusUpgrade()).To(ContainSubstring("RUNNING - Run pg_upgrade on primaries"))

		for i := range []int{0, 1, 2} {
			f, err := os.Create(filepath.Join(testStateDir, upgradestatus.CONVERT_PRIMARIES, fmt.Sprintf("seg%d", i), ".done"))
			Expect(err).ToNot(HaveOccurred())
			f.Write([]byte("Upgrade complete\n"))
			f.Close()
		}

		var pgUpgradeRan bool
		for _, cmd := range agentExecutor.LocalCommands {
			if strings.Contains(cmd, "/new/bindir/pg_upgrade") {
				pgUpgradeRan = true
				break
			}
		}
		Expect(pgUpgradeRan).To(BeTrue())

		// Return no PIDs when pgrep checks if pg_upgrade is running
		agentExecutor.LocalOutput = ""
		Expect(runStatusUpgrade()).To(ContainSubstring("COMPLETE - Run pg_upgrade on primaries"))
	})

	XIt("updates status to FAILED if convert primaries fails on at least 1 agent", func() {
		cm.AddStep(upgradestatus.CONVERT_PRIMARIES, idl.UpgradeSteps_CONVERT_PRIMARIES)

		Expect(runStatusUpgrade()).To(ContainSubstring("PENDING - Run pg_upgrade on primaries"))
		setStateFile(testStateDir, "convert-primaries/seg0", "1.failed")

		step := cm.GetStepWriter(upgradestatus.START_AGENTS)
		step.MarkInProgress()
		step.MarkComplete()

		upgradeConvertPrimaries := runCommand(
			"upgrade",
			"convert-primaries",
		)
		Expect(upgradeConvertPrimaries).Should(Exit(0))

		Expect(runStatusUpgrade()).To(ContainSubstring("FAILED - Run pg_upgrade on primaries"))
	})
})

func setStateFile(dir string, step string, state string) {
	err := os.MkdirAll(filepath.Join(dir, step), os.ModePerm)
	Expect(err).ToNot(HaveOccurred())

	f, err := os.Create(filepath.Join(dir, step, state))
	Expect(err).ToNot(HaveOccurred())
	f.Close()
}
