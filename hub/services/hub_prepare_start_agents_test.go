package services_test

import (
	"fmt"
	"os"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	_ "github.com/greenplum-db/gpupgrade/hub/services"

	"github.com/greenplum-db/gpupgrade/hub/services"

	"github.com/greenplum-db/gpupgrade/testutils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("hub PrepareStartAgents", func() {

	var (
		cp *services.ClusterPair
		cm *testutils.MockChecklistManager
	)

	BeforeEach(func() {
		cp = testutils.CreateSampleClusterPair()
		cm = testutils.NewMockChecklistManager()
	})

	It("shells out to cluster and runs gpupgrade_agent", func() {
		cp.OldCluster = testutils.CreateMultinodeSampleCluster()
		testExecutor := &testhelper.TestExecutor{}
		testExecutor.ClusterOutput = &cluster.RemoteOutput{}
		cp.OldCluster.Executor = testExecutor

		services.StartAgents(cp, cm)

		Expect(testExecutor.NumExecutions).To(Equal(1))

		startAgentsCmd := fmt.Sprintf("%s/bin/gpupgrade_agent --daemonize", os.Getenv("GPHOME"))
		clusterCommands := testExecutor.ClusterCommands[0]
		for _, command := range clusterCommands {
			Expect(command).To(ContainElement(startAgentsCmd))
		}
	})
})
