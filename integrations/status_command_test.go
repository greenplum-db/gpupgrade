package integrations_test

import (
	"os"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	agentServices "github.com/greenplum-db/gpupgrade/agent/services"
	hubServices "github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"

	"github.com/onsi/gomega/gbytes"
	"google.golang.org/grpc"

	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("status", func() {
	var (
		hub   *hubServices.Hub
		agent *agentServices.AgentServer
		cm    *testutils.MockChecklistManager
	)

	BeforeEach(func() {
		agentPort, err := testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		agentConf := agentServices.AgentConfig{
			Port:     agentPort,
			StateDir: testStateDir,
		}

		agentExecutor := &testhelper.TestExecutor{}
		agent = agentServices.NewAgentServer(agentExecutor, agentConf)
		go agent.Start()

		port, err = testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		conf := &hubServices.HubConfig{
			CliToHubPort:   port,
			HubToAgentPort: agentPort,
			StateDir:       testStateDir,
		}

		cm = testutils.NewMockChecklistManager()
		hub = hubServices.NewHub(testutils.InitClusterPairFromDB(), grpc.DialContext, conf, cm)
		go hub.Start()
	})

	AfterEach(func() {
		hub.Stop()
		agent.Stop()
		Expect(checkPortIsAvailable(port)).To(BeTrue())
	})

	Describe("conversion", func() {
		It("Displays status information for all segments", func() {
			pathToSegUpgrade := filepath.Join(testStateDir, "pg_upgrade", "seg-0")
			err := os.MkdirAll(pathToSegUpgrade, 0700)
			Expect(err).ToNot(HaveOccurred())

			f, err := os.Create(filepath.Join(pathToSegUpgrade, "1.done"))
			Expect(err).ToNot(HaveOccurred())
			f.WriteString("Upgrade complete\n")
			f.Close()

			statusSession := runCommand("status", "conversion")
			Eventually(statusSession).Should(Exit(0))

			Eventually(statusSession).Should(gbytes.Say("PENDING - DBID 1 - CONTENT ID -1 - MASTER - .+"))
			Eventually(statusSession).Should(gbytes.Say("COMPLETE - DBID 2 - CONTENT ID 0 - PRIMARY - .+"))
		})
	})

	// FIXME: The LoadSteps() method is ugly. It kind of proves that this should
	// be an end-to-end acceptance test, which ensures that `status upgrade`
	// does something actually useful.
	Describe("upgrade", func() {
		It("Reports status from the hub Checklist", func() {
			cm.LoadSteps([]upgradestatus.Step{
				{upgradestatus.CONFIG, pb.UpgradeSteps_CONFIG, nil},
				{upgradestatus.SEGINSTALL, pb.UpgradeSteps_SEGINSTALL, nil},
			})

			statusSession := runCommand("status", "upgrade")
			Eventually(statusSession).Should(Exit(0))

			Eventually(statusSession).Should(gbytes.Say("PENDING - Configuration Check"))
			Eventually(statusSession).Should(gbytes.Say("PENDING - Install binaries on segments"))
		})
	})
})
