package integrations_test

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
	. "github.com/onsi/gomega/gexec"
	"google.golang.org/grpc"
)

var _ = Describe("prepare start-agents", func() {
	var (
		hub *services.Hub
		cm  *testutils.MockChecklistManager
		cp  *services.ClusterPair
		err error
	)

	BeforeEach(func() {
		// The function runCommand depends on this port
		port, err = testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		conf := &services.HubConfig{
			CliToHubPort: port,
			StateDir:     testStateDir,
		}

		cp = testutils.CreateSampleClusterPair()
		cm = testutils.NewMockChecklistManager()

		hub = services.NewHub(cp, grpc.DialContext, nil, conf, nil, cm)
		go hub.Start()
	})

	AfterEach(func() {
		hub.Stop()
	})

	It("updates status PENDING to RUNNING then to COMPLETE if successful", func() {
		cp.OldCluster = testutils.CreateMultinodeSampleCluster()
		testExecutor := &testhelper.TestExecutor{}
		testExecutor.ClusterOutput = &cluster.RemoteOutput{}
		cp.OldCluster.Executor = testExecutor

		Expect(cm.IsPending(upgradestatus.START_AGENTS)).To(BeTrue())

		prepareStartAgentsSession := runCommand("prepare", "start-agents")
		Eventually(prepareStartAgentsSession).Should(Exit(0))

		// These assertions are identical to the ones in the prepare_start_agent unit tests but just to be safe we are leaving it in.
		Expect(testExecutor.NumExecutions).To(Equal(1))

		startAgentsCmd := fmt.Sprintf("%s/bin/gpupgrade_agent --daemonize", os.Getenv("GPHOME"))
		clusterCommands := testExecutor.ClusterCommands[0]
		for _, command := range clusterCommands {
			Expect(command).To(ContainElement(startAgentsCmd))
		}

		Expect(cm.IsComplete(upgradestatus.START_AGENTS)).To(BeTrue())
	})
})
