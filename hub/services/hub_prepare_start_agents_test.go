package services_test

import (
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"

	"github.com/greenplum-db/gpupgrade/testutils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("hub PrepareStartAgents", func() {
	It("shells out to cluster and runs gpupgrade_agent", func() {
		source, _ := testutils.CreateMultinodeSampleClusterPair("/tmp")
		testExecutor := &testhelper.TestExecutor{}
		testExecutor.ClusterOutput = &cluster.RemoteOutput{}
		source.Executor = testExecutor

		step := cm.GetStepWriter(upgradestatus.START_AGENTS)
		step.MarkInProgress()
		services.StartAgents(source, step)

		Expect(testExecutor.NumExecutions).To(Equal(1))
		Expect(cm.IsComplete(upgradestatus.START_AGENTS)).To(BeTrue())

		startAgentsCmd := fmt.Sprintf("%s/gpupgrade_agent --daemonize", source.BinDir)
		clusterCommands := testExecutor.ClusterCommands[0]
		for _, command := range clusterCommands {
			Expect(command).To(ContainElement(startAgentsCmd))
		}
	})
})
