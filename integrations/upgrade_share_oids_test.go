package integrations_test

import (
	"errors"
	"time"

	agentServices "github.com/greenplum-db/gpupgrade/agent/services"
	"github.com/greenplum-db/gpupgrade/hub/cluster_ssher"
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
		hubExecer *testutils.FakeCommandExecer
		agentPort int

		outChan chan []byte
		errChan chan error
		cm      *testutils.MockChecklistManager
	)

	BeforeEach(func() {
		var err error

		agentPort, err = testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		agentConfig := agentServices.AgentConfig{
			Port:     agentPort,
			StateDir: testStateDir,
		}

		agentExecer := &testutils.FakeCommandExecer{}
		agentExecer.SetOutput(&testutils.FakeCommand{})

		agent = agentServices.NewAgentServer(agentExecer.Exec, agentConfig)
		go agent.Start()

		port, err = testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		conf := &hubServices.HubConfig{
			CliToHubPort:   port,
			HubToAgentPort: agentPort,
			StateDir:       testStateDir,
		}

		outChan = make(chan []byte, 10)
		errChan = make(chan error, 10)
		hubExecer = &testutils.FakeCommandExecer{}
		hubExecer.SetOutput(&testutils.FakeCommand{
			Out: outChan,
			Err: errChan,
		})

		cm = testutils.NewMockChecklistManager()
		clusterSsher := cluster_ssher.NewClusterSsher(
			cm,
			hubServices.NewPingerManager(conf.StateDir, 500*time.Millisecond),
			hubExecer.Exec,
		)
		hub = hubServices.NewHub(testutils.InitClusterPairFromDB(), grpc.DialContext, hubExecer.Exec, conf, clusterSsher, cm)
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

		Expect(hubExecer.Calls()[0]).To(ContainSubstring("rsync"))
		Expect(cm.IsComplete(upgradestatus.SHARE_OIDS)).To(BeTrue())

	})

	It("updates status to FAILED if it fails to run", func() {

		Expect(cm.IsPending(upgradestatus.SHARE_OIDS)).To(BeTrue())
		errChan <- errors.New("fake test error, share oid failed to send files")

		upgradeShareOidsSession := runCommand("upgrade", "share-oids")
		Eventually(upgradeShareOidsSession).Should(Exit(0))
		Expect(cm.IsFailed(upgradestatus.SHARE_OIDS)).To(BeTrue())
	})
})
