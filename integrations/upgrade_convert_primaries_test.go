package integrations_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	agentServices "github.com/greenplum-db/gpupgrade/agent/services"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	"google.golang.org/grpc"
)

var _ = Describe("upgrade convert primaries", func() {
	var (
		hub           *services.Hub
		agent         *agentServices.AgentServer
		agentExecutor *testhelper.TestExecutor
		testExecutor  *testhelper.TestExecutor
		oidFile       string
		clusterPair   *utils.ClusterPair
		cm            *testutils.MockChecklistManager
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

		port, err = testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		conf := &services.HubConfig{
			CliToHubPort:   port,
			HubToAgentPort: 6416,
			StateDir:       testStateDir,
		}
		clusterPair = testutils.InitClusterPairFromDB()
		testExecutor = &testhelper.TestExecutor{}
		clusterPair.OldCluster.Executor = testExecutor
		hub = services.NewHub(clusterPair, grpc.DialContext, conf, cm)
		go hub.Start()

		agentExecutor = &testhelper.TestExecutor{}
		agent = agentServices.NewAgentServer(agentExecutor, agentServices.AgentConfig{
			Port:     6416,
			StateDir: testStateDir,
		})
		setStateFile(testStateDir, "start-agents", "completed")
		go agent.Start()
	})

	AfterEach(func() {
		hub.Stop()
		agent.Stop()
		utils.InitializeSystemFunctions()
		Expect(checkPortIsAvailable(port)).To(BeTrue())
	})

	// TODO: update to use MockChecklistManager.
	It("updates status PENDING to RUNNING then to COMPLETE if successful", func() {
		utils.System.RunCommandAsync = func(cmdStr string, logFile string) error {
			_, err := agentExecutor.ExecuteLocalCommand(cmdStr)
			return err
		}
		Expect(runStatusUpgrade()).To(ContainSubstring("PENDING - Primary segment upgrade"))
		testExecutor.LocalOutput = "TEST"

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

	It("updates status to FAILED if it fails to run", func() {
		Expect(runStatusUpgrade()).To(ContainSubstring("PENDING - Primary segment upgrade"))
		setStateFile(testStateDir, "pg_upgrade/seg-0", "1.failed")

		upgradeConvertPrimaries := runCommand(
			"upgrade",
			"convert-primaries",
			"--old-bindir", "/old/bindir",
			"--new-bindir", "/new/bindir",
		)
		Expect(upgradeConvertPrimaries).Should(Exit(0))

		Expect(runStatusUpgrade()).To(ContainSubstring("FAILED - Primary segment upgrade"))
	})

	It("fails if the --old-bindir or --new-bindir flags are missing", func() {
		upgradeConvertPrimaries := runCommand("upgrade", "convert-primaries")
		Expect(upgradeConvertPrimaries).Should(Exit(1))
		Expect(string(upgradeConvertPrimaries.Out.Contents())).To(Equal("Required flag(s) \"new-bindir\", \"old-bindir\" have/has not been set\n"))
	})
})

func setStateFile(dir string, step string, state string) {
	err := os.MkdirAll(filepath.Join(dir, step), os.ModePerm)
	Expect(err).ToNot(HaveOccurred())

	f, err := os.Create(filepath.Join(dir, step, state))
	Expect(err).ToNot(HaveOccurred())
	f.Close()
}
