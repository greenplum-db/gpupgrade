package integrations_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	agentServices "github.com/greenplum-db/gpupgrade/agent/services"
	"github.com/greenplum-db/gpupgrade/hub/cluster_ssher"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
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
		oldBinDir          string
		newBinDir          string
		oidFile            string
		hubOutChan         chan []byte
		agentOutChan       chan []byte
		clusterPair        *services.ClusterPair
	)

	BeforeEach(func() {
		var err error
		oldBinDir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
		newBinDir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		segmentDataDir := os.Getenv("MASTER_DATA_DIRECTORY")
		Expect(port).ToNot(Equal(""), "MASTER_DATA_DIRECTORY needs to be set!")

		config := fmt.Sprintf(`[{
			"content": 1,
			"dbid": 2,
			"hostname": "localhost",
			"datadir": "%s",
			"mode": "s",
			"preferred_role": "p",
			"role": "p",
			"status": "u",
			"port": 12345
		}]`, segmentDataDir)

		testutils.WriteOldConfig(testStateDir, config)
		testutils.WriteNewConfig(testStateDir, config)

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
			upgradestatus.NewChecklistManager(conf.StateDir),
			services.NewPingerManager(conf.StateDir, 500*time.Millisecond),
			hubCommandExecer.Exec,
		)
		clusterPair = testutils.InitClusterPairFromDB()
		hub = services.NewHub(clusterPair, grpc.DialContext, hubCommandExecer.Exec, conf, clusterSsher)
		go hub.Start()

		agentOutChan = make(chan []byte, 10)

		agentCommandExecer = &testutils.FakeCommandExecer{}
		agentCommandExecer.SetOutput(&testutils.FakeCommand{
			Out: agentOutChan,
		})
		agent = agentServices.NewAgentServer(agentCommandExecer.Exec, agentServices.AgentConfig{
			Port:     6416,
			StateDir: testStateDir,
		})
		go agent.Start()
	})

	AfterEach(func() {
		hub.Stop()
		agent.Stop()
		Expect(checkPortIsAvailable(port)).To(BeTrue())
	})

	// The primary status conversion logic is borked and returning master status instead
	// TODO: Fix status checking logic
	XIt("updates status PENDING to RUNNING then to COMPLETE if successful", func() {
		//Expect(clusterPair).To(BeNil())
		Expect(runStatusUpgrade()).To(ContainSubstring("PENDING - Primary segment upgrade"))
		hubOutChan <- []byte("TEST")

		agentOutChan <- []byte("combined output")
		agentOutChan <- []byte("pid1")

		upgradeConvertPrimaries := runCommand(
			"upgrade",
			"convert-primaries",
			"--old-bindir", oldBinDir,
			"--new-bindir", newBinDir,
		)
		Expect(upgradeConvertPrimaries).To(Exit(0))

		fmt.Println("Log:", string(testlog.Contents()))

		Expect(runStatusUpgrade()).To(ContainSubstring("RUNNING - Primary segment upgrade"))

		f, err := os.Create(filepath.Join(testStateDir, "pg_upgrade", "seg-1", ".done"))
		Expect(err).ToNot(HaveOccurred())
		f.Write([]byte("Upgrade complete\n"))
		f.Close()

		allCalls := strings.Join(agentCommandExecer.Calls(), "")
		Expect(allCalls).To(ContainSubstring(newBinDir + "/pg_upgrade"))

		Expect(runStatusUpgrade()).To(ContainSubstring("COMPLETE - Primary segment upgrade"))
	})

	It("updates status to FAILED if it fails to run", func() {
		Expect(runStatusUpgrade()).To(ContainSubstring("PENDING - Primary segment upgrade"))

		upgradeConvertPrimaries := runCommand(
			"upgrade",
			"convert-primaries",
			"--old-bindir", oldBinDir,
			"--new-bindir", newBinDir,
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
