package services_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
	"golang.org/x/net/context"

	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("status upgrade", func() {
	var (
		hub                      *services.Hub
		fakeStatusUpgradeRequest *pb.StatusUpgradeRequest
		dir                      string
		mockAgent                *testutils.MockAgentServer
		source                   *utils.Cluster
		target                   *utils.Cluster
		testExecutor             *testhelper.TestExecutor
		cm                       *testutils.MockChecklistManager
	)

	BeforeEach(func() {
		var port int
		mockAgent, port = testutils.NewMockAgentServer()
		mockAgent.StatusConversionResponse = &pb.CheckConversionStatusReply{}

		var err error
		dir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
		conf := &services.HubConfig{
			HubToAgentPort: port,
			StateDir:       dir,
		}

		source, target = testutils.CreateSampleClusterPair()
		testExecutor = &testhelper.TestExecutor{}
		source.Executor = testExecutor

		// Mock so statusConversion doesn't need to wait 3 seconds before erroring out.
		mockDialer := func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
			return nil, errors.New("grpc dial err")
		}

		cm = testutils.NewMockChecklistManager()
		// XXX this is wrong
		cm.LoadSteps([]upgradestatus.Step{
			{upgradestatus.CONFIG, pb.UpgradeSteps_CONFIG, nil},
			{upgradestatus.INIT_CLUSTER, pb.UpgradeSteps_INIT_CLUSTER, nil},
			{upgradestatus.SEGINSTALL, pb.UpgradeSteps_SEGINSTALL, nil},
			{upgradestatus.SHUTDOWN_CLUSTERS, pb.UpgradeSteps_SHUTDOWN_CLUSTERS, nil},
			{upgradestatus.CONVERT_MASTER, pb.UpgradeSteps_CONVERT_MASTER, nil},
			{upgradestatus.START_AGENTS, pb.UpgradeSteps_START_AGENTS, nil},
			{upgradestatus.SHARE_OIDS, pb.UpgradeSteps_SHARE_OIDS, nil},
			{upgradestatus.VALIDATE_START_CLUSTER, pb.UpgradeSteps_VALIDATE_START_CLUSTER, nil},
			{upgradestatus.CONVERT_PRIMARIES, pb.UpgradeSteps_CONVERT_PRIMARIES, nil},
			{upgradestatus.RECONFIGURE_PORTS, pb.UpgradeSteps_RECONFIGURE_PORTS, nil},
		})

		hub = services.NewHub(source, target, mockDialer, conf, cm)
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		os.RemoveAll(dir)
	})

	It("responds with the statuses of the steps based on checklist state", func() {
		for _, name := range []string{upgradestatus.CONFIG, upgradestatus.SEGINSTALL, upgradestatus.START_AGENTS} {
			step := cm.GetStepWriter(name)
			step.MarkInProgress()
			step.MarkComplete()
		}

		step := cm.GetStepWriter(upgradestatus.SHARE_OIDS)
		step.MarkInProgress()
		step.MarkFailed()

		resp, err := hub.StatusUpgrade(nil, &pb.StatusUpgradeRequest{})
		Expect(err).To(BeNil())

		Expect(resp.ListOfUpgradeStepStatuses).To(ConsistOf(
			[]*pb.UpgradeStepStatus{
				{
					Step:   pb.UpgradeSteps_CONFIG,
					Status: pb.StepStatus_COMPLETE,
				}, {
					Step:   pb.UpgradeSteps_INIT_CLUSTER,
					Status: pb.StepStatus_PENDING,
				}, {
					Step:   pb.UpgradeSteps_SEGINSTALL,
					Status: pb.StepStatus_COMPLETE,
				}, {
					Step:   pb.UpgradeSteps_SHUTDOWN_CLUSTERS,
					Status: pb.StepStatus_PENDING,
				}, {
					Step:   pb.UpgradeSteps_CONVERT_MASTER,
					Status: pb.StepStatus_PENDING,
				}, {
					Step:   pb.UpgradeSteps_START_AGENTS,
					Status: pb.StepStatus_COMPLETE,
				}, {
					Step:   pb.UpgradeSteps_SHARE_OIDS,
					Status: pb.StepStatus_FAILED,
				}, {
					Step:   pb.UpgradeSteps_VALIDATE_START_CLUSTER,
					Status: pb.StepStatus_PENDING,
				}, {
					Step:   pb.UpgradeSteps_CONVERT_PRIMARIES,
					Status: pb.StepStatus_PENDING,
				}, {
					Step:   pb.UpgradeSteps_RECONFIGURE_PORTS,
					Status: pb.StepStatus_PENDING,
				},
			}))
	})

	// TODO: Get rid of these tests once full rewritten test coverage exists
	Describe("creates a reply", func() {
		It("sends status messages under good condition", func() {
			formulatedResponse, err := hub.StatusUpgrade(nil, fakeStatusUpgradeRequest)
			Expect(err).To(BeNil())
			countOfStatuses := len(formulatedResponse.GetListOfUpgradeStepStatuses())
			Expect(countOfStatuses).ToNot(BeZero())
		})

		It("reports that prepare start-agents is pending", func() {
			utils.System.FilePathGlob = func(string) ([]string, error) {
				return []string{"somefile"}, nil
			}

			var fakeStatusUpgradeRequest *pb.StatusUpgradeRequest

			formulatedResponse, err := hub.StatusUpgrade(nil, fakeStatusUpgradeRequest)
			Expect(err).To(BeNil())

			stepStatuses := formulatedResponse.GetListOfUpgradeStepStatuses()

			var stepStatusSaved *pb.UpgradeStepStatus
			for _, stepStatus := range stepStatuses {

				if stepStatus.GetStep() == pb.UpgradeSteps_START_AGENTS {
					stepStatusSaved = stepStatus
				}
			}
			Expect(stepStatusSaved.GetStep()).ToNot(BeZero())
			Expect(stepStatusSaved.GetStatus()).To(Equal(pb.StepStatus_PENDING))
		})

		It("reports that prepare start-agents is running and then complete", func() {
			// TODO this is no longer a really useful test.
			pollStatusUpgrade := func() *pb.UpgradeStepStatus {
				response, _ := hub.StatusUpgrade(nil, &pb.StatusUpgradeRequest{})

				stepStatuses := response.GetListOfUpgradeStepStatuses()

				var stepStatusSaved *pb.UpgradeStepStatus
				for _, stepStatus := range stepStatuses {

					if stepStatus.GetStep() == pb.UpgradeSteps_START_AGENTS {
						stepStatusSaved = stepStatus
					}
				}
				return stepStatusSaved
			}

			step := cm.GetStepWriter(upgradestatus.START_AGENTS)
			step.MarkInProgress()

			status := pollStatusUpgrade()
			Expect(status.GetStep()).ToNot(BeZero())
			Expect(status.GetStatus()).To(Equal(pb.StepStatus_RUNNING))

			step.MarkComplete()

			status = pollStatusUpgrade()
			Expect(status.GetStep()).ToNot(BeZero())
			Expect(status.GetStatus()).To(Equal(pb.StepStatus_COMPLETE))
		})
	})

	Describe("Status of ShutdownClusters", func() {
		It("We're sending the status of shutdown clusters", func() {
			formulatedResponse, err := hub.StatusUpgrade(nil, fakeStatusUpgradeRequest)
			Expect(err).To(BeNil())
			countOfStatuses := len(formulatedResponse.GetListOfUpgradeStepStatuses())
			Expect(countOfStatuses).ToNot(BeZero())
			found := false
			for _, v := range formulatedResponse.GetListOfUpgradeStepStatuses() {
				if pb.UpgradeSteps_SHUTDOWN_CLUSTERS == v.Step {
					found = true
				}
			}
			Expect(found).To(Equal(true))
		})
	})
})

func setStateFile(dir string, step string, state string) {
	err := os.MkdirAll(filepath.Join(dir, step), os.ModePerm)
	Expect(err).ToNot(HaveOccurred())

	f, err := os.Create(filepath.Join(dir, step, state))
	Expect(err).ToNot(HaveOccurred())
	f.Close()
}
