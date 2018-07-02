package integrations_test

import (
	"errors"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	agentServices "github.com/greenplum-db/gpupgrade/agent/services"
	hubServices "github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/testutils"

	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("upgrade share oids", func() {
	var (
		hub       *hubServices.Hub
		agent     *agentServices.AgentServer
		agentPort int

		testExecutor *testhelper.TestExecutor
		cm           *testutils.MockChecklistManager
	)

	BeforeEach(func() {
		var err error

		agentPort, err = testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		agentConfig := agentServices.AgentConfig{
			Port:     agentPort,
			StateDir: testStateDir,
		}

		agentExecutor := &testhelper.TestExecutor{}

		agent = agentServices.NewAgentServer(agentExecutor, agentConfig)
		go agent.Start()

		port, err = testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		conf := &hubServices.HubConfig{
			CliToHubPort:   port,
			HubToAgentPort: agentPort,
			StateDir:       testStateDir,
		}

		cp := testutils.InitClusterPairFromDB()
		testExecutor = &testhelper.TestExecutor{}
		cp.OldCluster.Executor = testExecutor
		cm = testutils.NewMockChecklistManager()
		hub = hubServices.NewHub(cp, grpc.DialContext, conf, cm)
		go hub.Start()
	})

	AfterEach(func() {
		hub.Stop()
		agent.Stop()

		Expect(checkPortIsAvailable(port)).To(BeTrue())
		Expect(checkPortIsAvailable(agentPort)).To(BeTrue())
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
