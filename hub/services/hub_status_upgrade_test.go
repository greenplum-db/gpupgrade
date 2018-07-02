package services_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/hub/services"
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
		clusterPair              *utils.ClusterPair
		testExecutor             *testhelper.TestExecutor
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

		clusterPair = testutils.CreateSampleClusterPair()
		testExecutor = &testhelper.TestExecutor{}
		clusterPair.OldCluster.Executor = testExecutor

		// Mock so statusConversion doesn't need to wait 3 seconds before erroring out.
		mockDialer := func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
			return nil, errors.New("grpc dial err")
		}
		hub = services.NewHub(clusterPair, mockDialer, conf, nil)
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		os.RemoveAll(dir)
	})

	// This is probably wonky because the convert primaries state check mechanism is
	// using the MASTERUPGRADE step when upgrading primaries and needs to be fixed
	It("responds with the statuses of the steps based on files on disk", func() {
		setStateFile(dir, "check-config", "completed")
		setStateFile(dir, "seginstall", "completed")
		setStateFile(dir, "start-agents", "completed")
		setStateFile(dir, "share-oids", "failed")

		mockAgent.StatusConversionResponse = &pb.CheckConversionStatusReply{
			Statuses: []string{"RUNNING", "PENDING"},
		}

		resp, err := hub.StatusUpgrade(nil, &pb.StatusUpgradeRequest{})
		Expect(err).To(BeNil())

		Expect(resp.ListOfUpgradeStepStatuses).To(ConsistOf(
			[]*pb.UpgradeStepStatus{
				{
					Step:   pb.UpgradeSteps_CHECK_CONFIG,
					Status: pb.StepStatus_COMPLETE,
				}, {
					Step:   pb.UpgradeSteps_PREPARE_INIT_CLUSTER,
					Status: pb.StepStatus_PENDING,
				}, {
					Step:   pb.UpgradeSteps_SEGINSTALL,
					Status: pb.StepStatus_COMPLETE,
				}, {
					Step:   pb.UpgradeSteps_STOPPED_CLUSTER,
					Status: pb.StepStatus_PENDING,
				}, {
					Step:   pb.UpgradeSteps_MASTERUPGRADE,
					Status: pb.StepStatus_PENDING,
				}, {
					Step:   pb.UpgradeSteps_PREPARE_START_AGENTS,
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

				if stepStatus.GetStep() == pb.UpgradeSteps_PREPARE_START_AGENTS {
					stepStatusSaved = stepStatus
				}
			}
			Expect(stepStatusSaved.GetStep()).ToNot(BeZero())
			Expect(stepStatusSaved.GetStatus()).To(Equal(pb.StepStatus_PENDING))
		})

		It("reports that prepare start-agents is running and then complete", func() {
			var numInvocations int
			utils.System.FilePathGlob = func(input string) ([]string, error) {
				numInvocations += 1
				if numInvocations == 1 {
					return []string{filepath.Join(filepath.Dir(input), "in.progress")}, nil
				}
				return []string{filepath.Join(filepath.Dir(input), "completed")}, nil
			}
			utils.System.Stat = func(name string) (os.FileInfo, error) {
				return nil, nil
			}
			pollStatusUpgrade := func() pb.StepStatus {
				response, _ := hub.StatusUpgrade(nil, &pb.StatusUpgradeRequest{})

				stepStatuses := response.GetListOfUpgradeStepStatuses()

				var stepStatusSaved *pb.UpgradeStepStatus
				for _, stepStatus := range stepStatuses {

					if stepStatus.GetStep() == pb.UpgradeSteps_PREPARE_START_AGENTS {
						stepStatusSaved = stepStatus
					}
				}
				return stepStatusSaved.GetStatus()

			}

			//Expect(stepStatusSaved.GetStep()).ToNot(BeZero())
			Eventually(pollStatusUpgrade).Should(Equal(pb.StepStatus_COMPLETE))
		})

		Context("master upgrade status checking requires check config to have been run", func() {
			BeforeEach(func() {
				setStateFile(dir, "check-config", "completed")
			})
			It("reports that master upgrade is pending when pg_upgrade dir does not exist", func() {
				utils.System.IsNotExist = func(error) bool {
					return true
				}

				formulatedResponse, err := hub.StatusUpgrade(nil, fakeStatusUpgradeRequest)
				Expect(err).To(BeNil())

				stepStatuses := formulatedResponse.GetListOfUpgradeStepStatuses()

				for _, stepStatus := range stepStatuses {
					if stepStatus.GetStep() == pb.UpgradeSteps_MASTERUPGRADE {
						Expect(stepStatus.GetStatus()).To(Equal(pb.StepStatus_PENDING))
					}
				}
			})

			It("reports that master upgrade is running when pg_upgrade/*.inprogress files exists", func() {
				testExecutor.LocalOutput = "stdout/stderr message"

				utils.System.IsNotExist = func(error) bool {
					return false
				}
				utils.System.FilePathGlob = func(name string) ([]string, error) {
					if strings.Contains(name, "check-config") {
						return []string{filepath.Join(dir, "check-config", "completed")}, nil
					} else if strings.Contains(name, "gpstop") {
						// Not relevant to this test directly, but makes the output correct when printing the status
						return []string{}, nil
					} else {
						return []string{filepath.Join(dir, "pg_upgrade", ".inprogress")}, nil
					}
				}

				formulatedResponse, err := hub.StatusUpgrade(nil, fakeStatusUpgradeRequest)
				Expect(err).To(BeNil())

				stepStatuses := formulatedResponse.GetListOfUpgradeStepStatuses()
				for _, stepStatus := range stepStatuses {
					if stepStatus.GetStep() == pb.UpgradeSteps_MASTERUPGRADE {
						Expect(stepStatus.GetStatus()).To(Equal(pb.StepStatus_RUNNING))
					}
				}
			})

			It("reports that master upgrade is done when no *.inprogress files exist in ~/.gpupgrade/pg_upgrade", func() {
				testExecutor.LocalOutput = "stdout/stderr message"
				testExecutor.LocalError = errors.New("bogus error")

				utils.System.IsNotExist = func(error) bool {
					return false
				}
				utils.System.FilePathGlob = func(name string) ([]string, error) {
					if strings.Contains(name, "check-config") {
						return []string{filepath.Join(dir, "check-config", "completed")}, nil
					} else if strings.Contains(name, "done") {
						return []string{filepath.Join(dir, "pg_upgrade", "done")}, nil
					} else {
						return nil, nil
					}
				}

				utils.System.Stat = func(filename string) (os.FileInfo, error) {
					if strings.Contains(filename, "done") {
						return &testutils.FakeFileInfo{}, nil
					}
					return nil, nil
				}

				utils.System.Open = func(name string) (*os.File, error) {
					// Temporarily create a file that we can read as a real file descriptor
					fd, err := ioutil.TempFile("/tmp", "hub_status_upgrade_test")
					Expect(err).To(BeNil())

					filename := fd.Name()
					fd.WriteString("12312312;Upgrade complete;\n")
					fd.Close()
					return os.Open(filename)

				}

				formulatedResponse, err := hub.StatusUpgrade(nil, fakeStatusUpgradeRequest)
				Expect(err).To(BeNil())

				stepStatuses := formulatedResponse.GetListOfUpgradeStepStatuses()

				for _, stepStatus := range stepStatuses {
					if stepStatus.GetStep() == pb.UpgradeSteps_MASTERUPGRADE {
						Expect(stepStatus.GetStatus()).To(Equal(pb.StepStatus_COMPLETE))
					}
				}
			})

			It("reports pg_upgrade has failed", func() {
				testExecutor.LocalOutput = "stdout/stderr message"
				testExecutor.LocalError = errors.New("bogus error")

				utils.System.IsNotExist = func(error) bool {
					return false
				}
				utils.System.FilePathGlob = func(glob string) ([]string, error) {
					if strings.Contains(glob, "check-config") {
						return []string{filepath.Join(dir, "check-config", "completed")}, nil
					} else if strings.Contains(glob, "inprogress") {
						return nil, errors.New("fake error")
					} else if strings.Contains(glob, "done") {
						return []string{"found something"}, nil
					}

					return nil, errors.New("test not configured for this glob")
				}

				utils.System.Open = func(name string) (*os.File, error) {
					// Temporarily create a file that we can read as a real file descriptor
					fd, err := ioutil.TempFile("/tmp", "hub_status_upgrade_test")
					Expect(err).To(BeNil())

					filename := fd.Name()
					fd.WriteString("12312312;Upgrade failed;\n")
					fd.Close()
					return os.Open(filename)

				}
				formulatedResponse, err := hub.StatusUpgrade(nil, fakeStatusUpgradeRequest)
				Expect(err).To(BeNil())

				stepStatuses := formulatedResponse.GetListOfUpgradeStepStatuses()

				for _, stepStatus := range stepStatuses {
					if stepStatus.GetStep() == pb.UpgradeSteps_MASTERUPGRADE {
						Expect(stepStatus.GetStatus()).To(Equal(pb.StepStatus_FAILED))
					}
				}
			})
		})
	})

	Describe("Status of PrepareNewClusterConfig", func() {
		It("marks this step pending if there's no new cluster config file", func() {
			utils.System.Stat = func(filename string) (os.FileInfo, error) {
				return nil, errors.New("cannot find file") /* This is normally a PathError */
			}
			stepStatus := hub.GetPrepareNewClusterConfigStatus()
			Expect(stepStatus.Step).To(Equal(pb.UpgradeSteps_PREPARE_INIT_CLUSTER))
			Expect(stepStatus.Status).To(Equal(pb.StepStatus_PENDING))
		})

		It("marks this step complete if there is a new cluster config file", func() {
			utils.System.Stat = func(filename string) (os.FileInfo, error) {
				return nil, nil
			}

			stepStatus := hub.GetPrepareNewClusterConfigStatus()
			Expect(stepStatus.Step).To(Equal(pb.UpgradeSteps_PREPARE_INIT_CLUSTER))
			Expect(stepStatus.Status).To(Equal(pb.StepStatus_COMPLETE))

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
				if pb.UpgradeSteps_STOPPED_CLUSTER == v.Step {
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
