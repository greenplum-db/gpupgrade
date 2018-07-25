package services_test

import (
	"fmt"
	"os"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"

	"github.com/greenplum-db/gpupgrade/testutils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("hub CheckSeginstall", func() {

	var (
		cm *testutils.MockChecklistManager
	)

	BeforeEach(func() {
		cm = testutils.NewMockChecklistManager()
	})

	It("shells out to cluster and verifies gpupgrade_agent is installed on master and hosts", func() {
		source, _ := testutils.CreateMultinodeSampleClusterPair("/tmp")
		testExecutor := &testhelper.TestExecutor{}
		testExecutor.ClusterOutput = &cluster.RemoteOutput{}
		source.Cluster.Executor = testExecutor

		step := cm.GetStepWriter(upgradestatus.SEGINSTALL)
		step.MarkInProgress()
		services.VerifyAgentsInstalled(source, step)

		Expect(testExecutor.NumExecutions).To(Equal(1))
		Expect(cm.IsComplete(upgradestatus.SEGINSTALL)).To(BeTrue())

		lsCmd := fmt.Sprintf("ls %s/bin/gpupgrade_agent", os.Getenv("GPHOME"))
		clusterCommands := testExecutor.ClusterCommands[0]
		for _, command := range clusterCommands {
			Expect(command).To(ContainElement(lsCmd))
		}
	})
})
