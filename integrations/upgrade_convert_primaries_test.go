package integrations_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	agentServices "github.com/greenplum-db/gpupgrade/agent/services"
	"github.com/greenplum-db/gpupgrade/hub/cluster_ssher"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	"google.golang.org/grpc"
)

var _ = Describe("upgrade convert primaries", func() {
	var (
		hub                *services.Hub
		agent              *agentServices.AgentServer
		hubCommandExecer   *testutils.FakeCommandExecer
		agentCommandExecer *testutils.FakeCommandExecer
		oidFile            string
		hubOutChan         chan []byte
		agentCommandOutput chan []byte
		clusterPair        *services.ClusterPair
		cm                 *testutils.MockChecklistManager
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
		hubOutChan = make(chan []byte, 10)

		hubCommandExecer = &testutils.FakeCommandExecer{}
		hubCommandExecer.SetOutput(&testutils.FakeCommand{
			Out: hubOutChan,
		})

		clusterSsher := cluster_ssher.NewClusterSsher(
			cm,
			services.NewPingerManager(conf.StateDir, 500*time.Millisecond),
			hubCommandExecer.Exec,
		)
		clusterPair = testutils.InitClusterPairFromDB()
		hub = services.NewHub(clusterPair, grpc.DialContext, hubCommandExecer.Exec, conf, clusterSsher, cm)
		go hub.Start()

		agentCommandOutput = make(chan []byte, 12)

		agentCommandExecer = &testutils.FakeCommandExecer{}
		agentCommandExecer.SetOutput(&testutils.FakeCommand{
			Out: agentCommandOutput,
		})
		agent = agentServices.NewAgentServer(agentCommandExecer.Exec, agentServices.AgentConfig{
			Port:     6416,
			StateDir: testStateDir,
		})
		setStateFile(testStateDir, "start-agents", "completed")
		go agent.Start()
	})

	AfterEach(func() {
		hub.Stop()
		agent.Stop()
		Expect(checkPortIsAvailable(port)).To(BeTrue())
	})

	It("updates status PENDING to RUNNING then to COMPLETE if successful", func() {
		Expect(runStatusUpgrade()).To(ContainSubstring("PENDING - Primary segment upgrade"))
		hubOutChan <- []byte("TEST")

		agentCommandOutput <- []byte("run pg_upgrade for segment 0")
		agentCommandOutput <- []byte("run pg_upgrade for segment 1")
		agentCommandOutput <- []byte("run pg_upgrade for segment 2")

		upgradeConvertPrimaries := runCommand(
			"upgrade",
			"convert-primaries",
			"--old-bindir", "/old/bindir",
			"--new-bindir", "/new/bindir",
		)
		Expect(upgradeConvertPrimaries).To(Exit(0))

		agentCommandOutput <- []byte("pgrep for running pg_upgrade for segment 0")
		agentCommandOutput <- []byte("pgrep for running pg_upgrade for segment 1")
		agentCommandOutput <- []byte("pgrep for running pg_upgrade for segment 2")
		Expect(runStatusUpgrade()).To(ContainSubstring("RUNNING - Primary segment upgrade"))

		for i := range []int{0, 1, 2} {
			f, err := os.Create(filepath.Join(testStateDir, "pg_upgrade", fmt.Sprintf("seg-%d", i), ".done"))
			Expect(err).ToNot(HaveOccurred())
			f.Write([]byte("Upgrade complete\n"))
			f.Close()
		}

		allCalls := strings.Join(agentCommandExecer.Calls(), " ")
		Expect(allCalls).To(ContainSubstring(newBinDir + "/pg_upgrade"))

		// Return no PIDs when pgrep checks if pg_upgrade is running
		agentCommandOutput <- []byte("")
		agentCommandOutput <- []byte("")
		agentCommandOutput <- []byte("")
		Expect(runStatusUpgrade()).To(ContainSubstring("COMPLETE - Primary segment upgrade"))
	})

	It("updates status to FAILED if it fails to run", func() {
		Expect(runStatusUpgrade()).To(ContainSubstring("PENDING - Primary segment upgrade"))
		setStateFile(testStateDir, "pg_upgrade/seg-0", "1.failed")
		agentCommandOutput <- []byte("combined output")

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
