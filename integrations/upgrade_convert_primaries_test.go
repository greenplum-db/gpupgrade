package integrations_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("upgrade convert primaries", func() {
	var (
		oidFile string
	)

	BeforeEach(func() {
		var err error
		segmentDataDir := os.Getenv("MASTER_DATA_DIRECTORY")
		Expect(segmentDataDir).ToNot(Equal(""), "MASTER_DATA_DIRECTORY needs to be set!")

		err = os.MkdirAll(filepath.Join(testStateDir, "pg_upgrade"), 0700)
		Expect(err).ToNot(HaveOccurred())

		oidFile = filepath.Join(testStateDir, "pg_upgrade", "pg_upgrade_dump_seg1_oids.sql")
		f, err := os.Create(oidFile)
		Expect(err).ToNot(HaveOccurred())
		f.Close()

		go agent.Start()
	})

	// Move this elsewhere; it's not testing what's useful anymore.
	XIt("updates status PENDING to RUNNING then to COMPLETE if successful", func() {
		utils.System.RunCommandAsync = func(cmdStr string, logFile string) error {
			_, err := agentExecutor.ExecuteLocalCommand(cmdStr)
			return err
		}

		cm.LoadSteps([]upgradestatus.Step{
			{upgradestatus.CONVERT_PRIMARIES, pb.UpgradeSteps_CONVERT_PRIMARIES, nil},
		})

		Expect(runStatusUpgrade()).To(ContainSubstring("PENDING - Primary segment upgrade"))
		testExecutor.LocalOutput = "TEST"

		step := cm.GetStepWriter(upgradestatus.START_AGENTS)
		step.MarkInProgress()
		step.MarkComplete()

		agentExecutor.LocalOutput = "run pg_upgrade for segment"

		upgradeConvertPrimaries := runCommand(
			"upgrade",
			"convert-primaries",
			"--old-bindir", "/old/bindir",
			"--new-bindir", "/new/bindir",
		)
		Expect(upgradeConvertPrimaries).To(Exit(0))

		agentExecutor.LocalOutput = "pgrep for running pg_upgrade for segment"
		Expect(runStatusUpgrade()).To(ContainSubstring("RUNNING - Primary segment upgrade"))

		for i := range []int{0, 1, 2} {
			f, err := os.Create(filepath.Join(testStateDir, "pg_upgrade", fmt.Sprintf("seg-%d", i), ".done"))
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
		Expect(runStatusUpgrade()).To(ContainSubstring("COMPLETE - Primary segment upgrade"))
	})

	// Move this elsewhere; it's not testing what's useful anymore.
	XIt("updates status to FAILED if convert primaries fails on at least 1 agent", func() {
		cm.LoadSteps([]upgradestatus.Step{
			{upgradestatus.CONVERT_PRIMARIES, pb.UpgradeSteps_CONVERT_PRIMARIES, nil},
		})

		Expect(runStatusUpgrade()).To(ContainSubstring("PENDING - Primary segment upgrade"))
		setStateFile(testStateDir, "pg_upgrade/seg-0", "1.failed")

		step := cm.GetStepWriter(upgradestatus.START_AGENTS)
		step.MarkInProgress()
		step.MarkComplete()

		upgradeConvertPrimaries := runCommand(
			"upgrade",
			"convert-primaries",
			"--old-bindir", "/old/bindir",
			"--new-bindir", "/new/bindir",
		)
		Expect(upgradeConvertPrimaries).Should(Exit(0))

		Expect(runStatusUpgrade()).To(ContainSubstring("FAILED - Primary segment upgrade"))
	})
})

func setStateFile(dir string, step string, state string) {
	err := os.MkdirAll(filepath.Join(dir, step), os.ModePerm)
	Expect(err).ToNot(HaveOccurred())

	f, err := os.Create(filepath.Join(dir, step, state))
	Expect(err).ToNot(HaveOccurred())
	f.Close()
}
