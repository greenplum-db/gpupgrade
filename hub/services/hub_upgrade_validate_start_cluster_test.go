package services_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/greenplum-db/gpupgrade/hub/configutils"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"

	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/greenplum-db/gpupgrade/utils"
)

var _ = Describe("upgrade validate start cluster", func() {
	var (
		hub           *services.Hub
		reader        configutils.Reader
		dir           string
		commandExecer *testutils.FakeCommandExecer
		errChan       chan error
		outChan       chan []byte
		stubRemoteExecutor *testutils.StubRemoteExecutor
	)

	BeforeEach(func() {
		reader = configutils.NewReader()
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

		stubRemoteExecutor = testutils.NewStubRemoteExecutor()
		hub = services.NewHub(nil, &reader, grpc.DialContext, commandExecer.Exec, &services.HubConfig{
			StateDir: dir,
		}, stubRemoteExecutor)
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		os.RemoveAll(dir)
	})

	It("sets status to COMPLETE when validate start cluster request has been made and returns no error", func() {
		stateChecker := upgradestatus.NewStateCheck(
			filepath.Join(dir, "validate-start-cluster"),
			pb.UpgradeSteps_VALIDATE_START_CLUSTER,
		)

		trigger := make(chan struct{}, 1)
		commandExecer.SetTrigger(trigger)

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

		_, err := hub.UpgradeValidateStartCluster(nil, &pb.UpgradeValidateStartClusterRequest{
			NewBinDir:  "bin",
			NewDataDir: "data",
		})
		Expect(err).ToNot(HaveOccurred())
		wg.Wait()

		Expect(commandExecer.Command()).To(Equal("bash"))
		Expect(strings.Join(commandExecer.Args(), "")).To(ContainSubstring("PYTHONPATH="))
		Expect(strings.Join(commandExecer.Args(), "")).To(ContainSubstring("&& bin/gpstart -a -d data"))

		Eventually(stateChecker.GetStatus).Should(Equal(&pb.UpgradeStepStatus{
			Step:   pb.UpgradeSteps_VALIDATE_START_CLUSTER,
			Status: pb.StepStatus_COMPLETE,
		}))
	})

	It("sets status to FAILED when the validate start cluster request returns an error", func() {
		errChan <- errors.New("some error")

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
