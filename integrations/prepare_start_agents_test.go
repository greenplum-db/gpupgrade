package integrations_test

import (
	"os"
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

var _ = Describe("prepare", func() {
	var (
		hub           *services.Hub
		mockAgent     *testutils.MockAgentServer
		commandExecer *testutils.FakeCommandExecer
		cm            *testutils.MockChecklistManager
	)

	BeforeEach(func() {
		var agentPort int
		mockAgent, agentPort = testutils.NewMockAgentServer()

		var err error
		port, err = testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		conf := &services.HubConfig{
			CliToHubPort:   port,
			HubToAgentPort: agentPort,
			StateDir:       testStateDir,
		}
		commandExecer = &testutils.FakeCommandExecer{}
		commandExecer.SetOutput(&testutils.FakeCommand{})

		cm = testutils.NewMockChecklistManager()
		clusterSsher := cluster_ssher.NewClusterSsher(
			cm,
			services.NewPingerManager(conf.StateDir, 500*time.Millisecond),
			commandExecer.Exec,
		)
		hub = services.NewHub(testutils.InitClusterPairFromDB(), grpc.DialContext, commandExecer.Exec, conf, clusterSsher, cm)

		pgPort := os.Getenv("PGPORT")
		Expect(pgPort).ToNot(Equal(""), "Please set PGPORT to a useful value and rerun the tests.")

		go hub.Start()
	})

	AfterEach(func() {
		hub.Stop()
		mockAgent.Stop()
		Expect(checkPortIsAvailable(port)).To(BeTrue())
	})

	Describe("start-agents", func() {
		It("updates status PENDING to RUNNING then to COMPLETE if successful", func(done Done) {
			defer close(done)

			mockAgent.StatusConversionResponse = &pb.CheckConversionStatusReply{
				Statuses: []string{},
			}

			Expect(cm.IsPending(upgradestatus.START_AGENTS)).To(BeTrue())

			prepareStartAgentsSession := runCommand("prepare", "start-agents")
			Eventually(prepareStartAgentsSession).Should(Exit(0))

			Expect(commandExecer.Command()).To(Equal("ssh"))
			Expect(strings.Join(commandExecer.Args(), "")).To(ContainSubstring("nohup"))
			Expect(cm.IsComplete(upgradestatus.START_AGENTS)).To(BeTrue())
		})
	})
})
