package integrations_test

import (
	"strings"
	"time"

	"github.com/greenplum-db/gpupgrade/hub/cluster_ssher"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	"google.golang.org/grpc"
)

var _ = Describe("check", func() {
	var (
		hub           *services.Hub
		mockAgent     *testutils.MockAgentServer
		commandExecer *testutils.FakeCommandExecer
		outChan       chan []byte
		errChan       chan error
		cm            *testutils.MockChecklistManager
	)

	BeforeEach(func() {
		var err error

		port, err = testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		var agentPort int
		mockAgent, agentPort = testutils.NewMockAgentServer()

		conf := &services.HubConfig{
			CliToHubPort:   port,
			HubToAgentPort: agentPort,
			StateDir:       testStateDir,
		}
		outChan = make(chan []byte, 2)
		errChan = make(chan error, 2)

		commandExecer = &testutils.FakeCommandExecer{}
		commandExecer.SetOutput(&testutils.FakeCommand{
			Out: outChan,
			Err: errChan,
		})

		cm = testutils.NewMockChecklistManager()
		clusterSsher := cluster_ssher.NewClusterSsher(
			cm,
			services.NewPingerManager(conf.StateDir, 500*time.Millisecond),
			commandExecer.Exec,
		)

		hub = services.NewHub(testutils.InitClusterPairFromDB(), grpc.DialContext, commandExecer.Exec, conf, clusterSsher, cm)
		go hub.Start()
	})

	AfterEach(func() {
		hub.Stop()
		mockAgent.Stop()
	})

	// `gpupgrade check seginstall` verifies that the user has installed the software on all hosts
	// As a single-node check, this test verifies the mechanics of the check, but would typically succeed.
	// The implementation, however, uses the gpupgrade_agent binary to verify installation. In real life,
	// all the binaries, gpupgrade_hub and gpupgrade_agent included, would be alongside each other.
	// But in our integration tests' context, only the necessary Golang code is compiled, and Ginkgo's default
	// is to compile gpupgrade_hub and gpupgrade_agent in separate directories. As such, this test depends on the
	// setup in `integrations_suite_test.go` to replicate the real-world scenario of "install binaries side-by-side".
	//
	// TODO: This test might be interesting to run multi-node; for that, figure out how "installation" should be done
	Describe("seginstall", func() {
		It("updates status PENDING to RUNNING then to COMPLETE if successful", func() {
			mockAgent.StatusConversionResponse = &pb.CheckConversionStatusReply{
				Statuses: []string{},
			}

			Expect(cm.IsPending(upgradestatus.SEGINSTALL)).To(BeTrue())

			checkSeginstallSession := runCommand("check", "seginstall", "--master-host", "localhost")
			Eventually(checkSeginstallSession).Should(Exit(0))

			Expect(commandExecer.Command()).To(Equal("ssh"))
			Expect(strings.Join(commandExecer.Args(), "")).To(ContainSubstring("ls"))
			Expect(cm.IsComplete(upgradestatus.SEGINSTALL)).To(BeTrue())
		})
	})

	It("fails if the --master-host flag is missing", func() {
		checkSeginstallSession := runCommand("check", "seginstall")
		Expect(checkSeginstallSession).Should(Exit(1))
		Expect(string(checkSeginstallSession.Out.Contents())).To(Equal("Required flag(s) \"master-host\" have/has not been set\n"))
	})
})
