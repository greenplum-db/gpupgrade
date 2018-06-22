package integrations_test

import (
	"errors"

	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/testutils"

	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("upgrade reconfigure ports", func() {

	var (
		hub       *services.Hub
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

		port, err = testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		conf := &services.HubConfig{
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
		hub = services.NewHub(testutils.InitClusterPairFromDB(), grpc.DialContext, hubExecer.Exec, conf, cm)
		go hub.Start()
	})

	AfterEach(func() {
		hub.Stop()

		Expect(checkPortIsAvailable(port)).To(BeTrue())
		Expect(checkPortIsAvailable(agentPort)).To(BeTrue())
	})

	It("updates status PENDING to COMPLETE if successful", func() {
		Expect(cm.IsPending(upgradestatus.RECONFIGURE_PORTS)).To(BeTrue())

		upgradeReconfigurePortsSession := runCommand("upgrade", "reconfigure-ports")
		Eventually(upgradeReconfigurePortsSession).Should(Exit(0))

		Expect(hubExecer.Calls()[0]).To(ContainSubstring("sed"))

		Expect(cm.IsComplete(upgradestatus.RECONFIGURE_PORTS)).To(BeTrue())

	})

	It("updates status to FAILED if it fails to run", func() {

		Expect(cm.IsPending(upgradestatus.RECONFIGURE_PORTS)).To(BeTrue())
		errChan <- errors.New("fake test error, reconfigure-ports failed")

		upgradeShareOidsSession := runCommand("upgrade", "reconfigure-ports")
		Eventually(upgradeShareOidsSession).Should(Exit(1))
		Expect(cm.IsFailed(upgradestatus.RECONFIGURE_PORTS)).To(BeTrue())
	})
})
