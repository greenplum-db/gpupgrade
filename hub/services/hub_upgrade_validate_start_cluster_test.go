package services_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/hub/cluster_ssher"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"

	"google.golang.org/grpc"

	"github.com/greenplum-db/gpupgrade/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("upgrade validate start cluster", func() {
	var (
		hub           *services.Hub
		dir           string
		commandExecer *testutils.FakeCommandExecer
		errChan       chan error
		outChan       chan []byte
		clusterPair   *services.ClusterPair
		testExecutor  *testhelper.TestExecutor
		cm            *testutils.MockChecklistManager
	)

	BeforeEach(func() {
		var err error
		dir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		errChan = make(chan error, 1)
		outChan = make(chan []byte, 1)

		commandExecer = &testutils.FakeCommandExecer{}
		commandExecer.SetOutput(&testutils.FakeCommand{
			Err: errChan,
			Out: outChan,
		})

		clusterPair = testutils.CreateSampleClusterPair()
		testExecutor = &testhelper.TestExecutor{}
		clusterPair.NewCluster.Executor = testExecutor
		cm = testutils.NewMockChecklistManager()
		clusterSsher := cluster_ssher.NewClusterSsher(cm, nil, nil)
		hub = services.NewHub(clusterPair, grpc.DialContext, commandExecer.Exec, &services.HubConfig{
			StateDir: dir,
		}, clusterSsher, cm)
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		os.RemoveAll(dir)
	})

	It("sets status to COMPLETE when validate start cluster request has been made and returns no error", func() {
		Expect(cm.IsPending("validate-start-cluster")).To(BeTrue())

		_, err := hub.UpgradeValidateStartCluster(nil, &pb.UpgradeValidateStartClusterRequest{})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() bool { return cm.IsComplete("validate-start-cluster") }).Should(BeTrue())
		Expect(testExecutor.NumExecutions).To(Equal(1))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("source /new/bindir/../greenplum_path.sh"))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("/new/bindir/gpstart -a -d /new/datadir"))
	})

	It("sets status to FAILED when the validate start cluster request returns an error", func() {
		testExecutor.LocalError = errors.New("some error")

		_, err := hub.UpgradeValidateStartCluster(nil, &pb.UpgradeValidateStartClusterRequest{})
		Expect(err).ToNot(HaveOccurred())

		stateChecker := upgradestatus.NewStateCheck(
			filepath.Join(dir, "validate-start-cluster"),
			pb.UpgradeSteps_VALIDATE_START_CLUSTER,
		)

		Eventually(stateChecker.GetStatus).Should(Equal(&pb.UpgradeStepStatus{
			Step:   pb.UpgradeSteps_VALIDATE_START_CLUSTER,
			Status: pb.StepStatus_FAILED,
		}))
	})
})
