package services_test

import (
	"os"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("status upgrade", func() {
	var (
		fakeStatusUpgradeRequest *idl.StatusUpgradeRequest
		testExecutor             *testhelper.TestExecutor
	)

	BeforeEach(func() {
		mockAgent.StatusConversionResponse = &idl.CheckConversionStatusReply{}

		testExecutor = &testhelper.TestExecutor{}
		source.Executor = testExecutor

		cm = testutils.NewMockChecklistManager()
		cm.AddStep(upgradestatus.CONFIG, idl.UpgradeSteps_CONFIG)
		cm.AddStep(upgradestatus.INIT_CLUSTER, idl.UpgradeSteps_INIT_CLUSTER)
		cm.AddStep(upgradestatus.SEGINSTALL, idl.UpgradeSteps_SEGINSTALL)
		cm.AddStep(upgradestatus.SHUTDOWN_CLUSTERS, idl.UpgradeSteps_SHUTDOWN_CLUSTERS)
		cm.AddStep(upgradestatus.CONVERT_MASTER, idl.UpgradeSteps_CONVERT_MASTER)
		cm.AddStep(upgradestatus.START_AGENTS, idl.UpgradeSteps_START_AGENTS)
		cm.AddStep(upgradestatus.SHARE_OIDS, idl.UpgradeSteps_SHARE_OIDS)
		cm.AddStep(upgradestatus.VALIDATE_START_CLUSTER, idl.UpgradeSteps_VALIDATE_START_CLUSTER)
		cm.AddStep(upgradestatus.CONVERT_PRIMARIES, idl.UpgradeSteps_CONVERT_PRIMARIES)
		cm.AddStep(upgradestatus.RECONFIGURE_PORTS, idl.UpgradeSteps_RECONFIGURE_PORTS)

		hub = services.NewHub(source, target, dialer, hubConf, cm)
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

		resp, err := hub.StatusUpgrade(nil, &idl.StatusUpgradeRequest{})
		Expect(err).To(BeNil())

		Expect(resp.ListOfUpgradeStepStatuses).To(ConsistOf(
			[]*idl.UpgradeStepStatus{
				{
					Step:   idl.UpgradeSteps_CONFIG,
					Status: idl.StepStatus_COMPLETE,
				}, {
					Step:   idl.UpgradeSteps_INIT_CLUSTER,
					Status: idl.StepStatus_PENDING,
				}, {
					Step:   idl.UpgradeSteps_SEGINSTALL,
					Status: idl.StepStatus_COMPLETE,
				}, {
					Step:   idl.UpgradeSteps_SHUTDOWN_CLUSTERS,
					Status: idl.StepStatus_PENDING,
				}, {
					Step:   idl.UpgradeSteps_CONVERT_MASTER,
					Status: idl.StepStatus_PENDING,
				}, {
					Step:   idl.UpgradeSteps_START_AGENTS,
					Status: idl.StepStatus_COMPLETE,
				}, {
					Step:   idl.UpgradeSteps_SHARE_OIDS,
					Status: idl.StepStatus_FAILED,
				}, {
					Step:   idl.UpgradeSteps_VALIDATE_START_CLUSTER,
					Status: idl.StepStatus_PENDING,
				}, {
					Step:   idl.UpgradeSteps_CONVERT_PRIMARIES,
					Status: idl.StepStatus_PENDING,
				}, {
					Step:   idl.UpgradeSteps_RECONFIGURE_PORTS,
					Status: idl.StepStatus_PENDING,
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

			var fakeStatusUpgradeRequest *idl.StatusUpgradeRequest

			formulatedResponse, err := hub.StatusUpgrade(nil, fakeStatusUpgradeRequest)
			Expect(err).To(BeNil())

			stepStatuses := formulatedResponse.GetListOfUpgradeStepStatuses()

			var stepStatusSaved *idl.UpgradeStepStatus
			for _, stepStatus := range stepStatuses {

				if stepStatus.GetStep() == idl.UpgradeSteps_START_AGENTS {
					stepStatusSaved = stepStatus
				}
			}
			Expect(stepStatusSaved.GetStep()).ToNot(BeZero())
			Expect(stepStatusSaved.GetStatus()).To(Equal(idl.StepStatus_PENDING))
		})

		It("reports that prepare start-agents is running and then complete", func() {
			// TODO this is no longer a really useful test.
			pollStatusUpgrade := func() *idl.UpgradeStepStatus {
				response, _ := hub.StatusUpgrade(nil, &idl.StatusUpgradeRequest{})

				stepStatuses := response.GetListOfUpgradeStepStatuses()

				var stepStatusSaved *idl.UpgradeStepStatus
				for _, stepStatus := range stepStatuses {

					if stepStatus.GetStep() == idl.UpgradeSteps_START_AGENTS {
						stepStatusSaved = stepStatus
					}
				}
				return stepStatusSaved
			}

			step := cm.GetStepWriter(upgradestatus.START_AGENTS)
			step.MarkInProgress()

			status := pollStatusUpgrade()
			Expect(status.GetStep()).ToNot(BeZero())
			Expect(status.GetStatus()).To(Equal(idl.StepStatus_RUNNING))

			step.MarkComplete()

			status = pollStatusUpgrade()
			Expect(status.GetStep()).ToNot(BeZero())
			Expect(status.GetStatus()).To(Equal(idl.StepStatus_COMPLETE))
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
				if idl.UpgradeSteps_SHUTDOWN_CLUSTERS == v.Step {
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
