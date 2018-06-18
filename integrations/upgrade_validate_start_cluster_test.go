package integrations_test

import (
	"io/ioutil"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/hub/cluster_ssher"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

var _ = Describe("upgrade validate-start-cluster", func() {
	var (
		hub           *services.Hub
		commandExecer *testutils.FakeCommandExecer
		outChan       chan []byte
		errChan       chan error
		clusterPair   *services.ClusterPair
		testExecutor  *testhelper.TestExecutor
		cm            *testutils.MockChecklistManager
	)

	BeforeEach(func() {
		var err error

		port, err = testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		conf := &services.HubConfig{
			CliToHubPort:   port,
			HubToAgentPort: 6416,
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
		clusterPair = testutils.InitClusterPairFromDB()
		testExecutor = &testhelper.TestExecutor{}
		clusterPair.OldCluster.Executor = testExecutor
		hub = services.NewHub(clusterPair, grpc.DialContext, commandExecer.Exec, conf, clusterSsher, cm)
		go hub.Start()
	})

	AfterEach(func() {
		hub.Stop()
		Expect(checkPortIsAvailable(port)).To(BeTrue())
	})

	It("updates status PENDING to RUNNING then to COMPLETE if successful", func(done Done) {
		defer close(done)
		newBinDir, err := ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
		newDataDir, err := ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		Expect(cm.IsPending(upgradestatus.VALIDATE_START_CLUSTER)).To(BeTrue())

		session := runCommand("upgrade", "validate-start-cluster", "--new-bindir", newBinDir, "--new-datadir", newDataDir)
		Eventually(session).Should(Exit(0))

		Expect(testExecutor.NumExecutions).To(Equal(1))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("gpstart"))
		Expect(cm.IsPending(upgradestatus.VALIDATE_START_CLUSTER)).To(BeTrue())

	})

	It("updates status to FAILED if it fails to run", func() {
		newBinDir, err := ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
		newDataDir, err := ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		Expect(cm.IsPending(upgradestatus.VALIDATE_START_CLUSTER)).To(BeTrue())

		testExecutor.LocalError = errors.New("start failed")

		session := runCommand("upgrade", "validate-start-cluster", "--new-bindir", newBinDir, "--new-datadir", newDataDir)
		Eventually(session).Should(Exit(0))

		Expect(testExecutor.NumExecutions).To(Equal(1))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("gpstart"))
		Expect(cm.IsPending(upgradestatus.VALIDATE_START_CLUSTER)).To(BeTrue())
	})

	It("fails if the --new-bindir or --new-datadir flags are missing", func() {
		session := runCommand("upgrade", "validate-start-cluster")
		Expect(session).Should(Exit(1))
		Expect(string(session.Out.Contents())).To(Equal("Required flag(s) \"new-bindir\", \"new-datadir\" have/has not been set\n"))
	})
})
