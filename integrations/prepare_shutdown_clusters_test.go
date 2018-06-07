package integrations_test

import (
	"errors"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
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

var _ = Describe("prepare shutdown-clusters", func() {
	var (
		hub           *services.Hub
		mockAgent     *testutils.MockAgentServer
		commandExecer *testutils.FakeCommandExecer
		outChan       chan []byte
		errChan       chan error
		clusterPair   *services.ClusterPair
		testExecutor  *testhelper.TestExecutor
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
		outChan = make(chan []byte, 5)
		errChan = make(chan error, 5)

		commandExecer = &testutils.FakeCommandExecer{}
		commandExecer.SetOutput(&testutils.FakeCommand{
			Out: outChan,
			Err: errChan,
		})
		clusterPair = testutils.InitClusterPairFromDB()
		testExecutor = &testhelper.TestExecutor{}
		clusterPair.OldCluster.Executor = testExecutor
		clusterPair.OldBinDir = "/tmpOld"
		clusterPair.NewBinDir = "/tmpNew"
		clusterSsher := cluster_ssher.NewClusterSsher(
			upgradestatus.NewChecklistManager(conf.StateDir),
			services.NewPingerManager(conf.StateDir, 500*time.Millisecond),
			commandExecer.Exec,
		)
		hub = services.NewHub(clusterPair, grpc.DialContext, commandExecer.Exec, conf, clusterSsher)
		go hub.Start()
	})

	AfterEach(func() {
		hub.Stop()
		mockAgent.Stop()
	})

	It("updates status PENDING and then to COMPLETE if successful", func(done Done) {
		defer close(done)
		mockAgent.StatusConversionResponse = &pb.CheckConversionStatusReply{
			Statuses: []string{},
		}

		Expect(runStatusUpgrade()).To(ContainSubstring("PENDING - Shutdown clusters"))

		prepareShutdownClustersSession := runCommand("prepare", "shutdown-clusters", "--old-bindir", clusterPair.OldBinDir, "--new-bindir", clusterPair.NewBinDir)
		Eventually(prepareShutdownClustersSession).Should(Exit(0))

		Expect(testExecutor.NumExecutions).To(Equal(4))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("pgrep"))
		Expect(testExecutor.LocalCommands[1]).To(ContainSubstring("pgrep"))
		Expect(testExecutor.LocalCommands[2]).To(ContainSubstring(clusterPair.OldBinDir + "/gpstop -a"))
		Expect(testExecutor.LocalCommands[3]).To(ContainSubstring(clusterPair.NewBinDir + "/gpstop -a"))
		Eventually(runStatusUpgrade).Should(ContainSubstring("COMPLETE - Shutdown clusters"))
	})

	It("updates status to FAILED if it fails to run", func() {
		mockAgent.StatusConversionResponse = &pb.CheckConversionStatusReply{
			Statuses: []string{},
		}

		Expect(runStatusUpgrade()).To(ContainSubstring("PENDING - Shutdown clusters"))

		testExecutor.ErrorOnExecNum = 4
		testExecutor.LocalError = errors.New("start failed")

		prepareShutdownClustersSession := runCommand("prepare", "shutdown-clusters", "--old-bindir", clusterPair.OldBinDir, "--new-bindir", clusterPair.NewBinDir)
		Eventually(prepareShutdownClustersSession).Should(Exit(0))

		Expect(testExecutor.NumExecutions).To(Equal(4))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("pgrep"))
		Expect(testExecutor.LocalCommands[1]).To(ContainSubstring("pgrep"))
		Expect(testExecutor.LocalCommands[2]).To(ContainSubstring(clusterPair.OldBinDir + "/gpstop -a"))
		Expect(testExecutor.LocalCommands[3]).To(ContainSubstring(clusterPair.NewBinDir + "/gpstop -a"))
		Eventually(runStatusUpgrade).Should(ContainSubstring("FAILED - Shutdown clusters"))
	})

	It("fails if the --old-bindir or --new-bindir flags are missing", func() {
		prepareShutdownClustersSession := runCommand("prepare", "shutdown-clusters")
		Expect(prepareShutdownClustersSession).Should(Exit(1))
		Expect(string(prepareShutdownClustersSession.Out.Contents())).To(Equal("Required flag(s) \"new-bindir\", \"old-bindir\" have/has not been set\n"))
	})
})
