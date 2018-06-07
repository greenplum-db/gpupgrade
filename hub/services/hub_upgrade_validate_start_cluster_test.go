package services_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
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
		hub                *services.Hub
		dir                string
		commandExecer      *testutils.FakeCommandExecer
		errChan            chan error
		outChan            chan []byte
		stubRemoteExecutor *testutils.StubRemoteExecutor
		clusterPair        *services.ClusterPair
		testExecutor       *testhelper.TestExecutor
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
		clusterPair.OldCluster.Executor = testExecutor
		stubRemoteExecutor = testutils.NewStubRemoteExecutor()
		hub = services.NewHub(clusterPair, grpc.DialContext, commandExecer.Exec, &services.HubConfig{
			StateDir: dir,
		}, stubRemoteExecutor)
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		os.RemoveAll(dir)
	})

	//This test is flaking... It will be rewritten once the checklist manager refactor comes in.
	XIt("sets status to COMPLETE when validate start cluster request has been made and returns no error", func() {
		stateChecker := upgradestatus.NewStateCheck(
			filepath.Join(dir, "validate-start-cluster"),
			pb.UpgradeSteps_VALIDATE_START_CLUSTER,
		)

		trigger := make(chan struct{}, 1)
		commandExecer.SetTrigger(trigger)

		fmt.Println("1")
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer GinkgoRecover()

			Eventually(stateChecker.GetStatus).Should(Equal(&pb.UpgradeStepStatus{
				Step:   pb.UpgradeSteps_VALIDATE_START_CLUSTER,
				Status: pb.StepStatus_RUNNING,
			}))
			trigger <- struct{}{}
		}()
		fmt.Println("2")

		_, err := hub.UpgradeValidateStartCluster(nil, &pb.UpgradeValidateStartClusterRequest{
			NewBinDir:  "bin",
			NewDataDir: "data",
		})
		fmt.Println("3")
		Expect(err).ToNot(HaveOccurred())
		wg.Wait()

		fmt.Println("4")
		Expect(testExecutor.NumExecutions).To(Equal(1))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("PYTHONPATH="))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("&& bin/gpstart -a -d data"))

		fmt.Println("5")
		Eventually(stateChecker.GetStatus).Should(Equal(&pb.UpgradeStepStatus{
			Step:   pb.UpgradeSteps_VALIDATE_START_CLUSTER,
			Status: pb.StepStatus_COMPLETE,
		}))
		fmt.Println("6")
	})

	It("sets status to FAILED when the validate start cluster request returns an error", func() {
		testExecutor.LocalError = errors.New("some error")

		_, err := hub.UpgradeValidateStartCluster(nil, &pb.UpgradeValidateStartClusterRequest{
			NewBinDir:  "bin",
			NewDataDir: "data",
		})
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
