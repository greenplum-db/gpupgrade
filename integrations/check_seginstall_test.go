package integrations_test

import (
	"fmt"
	"os"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	"google.golang.org/grpc"
)

var _ = Describe("check seginstall", func() {
	var (
		hub *services.Hub
		cm  *testutils.MockChecklistManager
		cp  *utils.ClusterPair
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

		hub = services.NewHub(cp, grpc.DialContext, nil, conf, cm)
		go hub.Start()
	})

	AfterEach(func() {
		hub.Stop()
	})

	// `gpupgrade check seginstall` verifies that the user has installed the software on all hosts
	// As a single-node check, this test verifies the mechanics of the check, but would typically succeed.
	// The implementation, however, uses the gpupgrade_agent binary to verify installation. In real life,
	// all the binaries, gpupgrade_hub and gpupgrade_agent included, would be alongside each other.
	// But in our integration tests' context, only the necessary Golang code is compiled, and Ginkgo's default
	// is to compile gpupgrade_hub and gpupgrade_agent in separate directories. As such, this test depends on the
	// setup in `integrations_suite_test.go` to replicate the real-world scenario of "install binaries side-by-side".
	//
	// TODO: This test might be interesting to run multi-node; for that, figure out how "installation" should be done
	It("updates status PENDING to RUNNING then to COMPLETE if successful", func() {
		cp.OldCluster = testutils.CreateMultinodeSampleCluster()
		testExecutor := &testhelper.TestExecutor{}
		testExecutor.ClusterOutput = &cluster.RemoteOutput{}
		cp.OldCluster.Executor = testExecutor

		Expect(cm.IsPending(upgradestatus.SEGINSTALL)).To(BeTrue())

		checkSeginstallSession := runCommand("check", "seginstall", "--master-host", "localhost")
		Eventually(checkSeginstallSession).Should(Exit(0))

		// These assertions are identical to the ones in the hub_check_seginstall unit tests but just to be safe we are leaving it in.
		Expect(testExecutor.NumExecutions).To(Equal(1))

		lsCmd := fmt.Sprintf("ls %s/bin/gpupgrade_agent", os.Getenv("GPHOME"))
		clusterCommands := testExecutor.ClusterCommands[0]
		for _, command := range clusterCommands {
			Expect(command).To(ContainElement(lsCmd))
		}

		Expect(cm.IsComplete(upgradestatus.SEGINSTALL)).To(BeTrue())
	})

	It("fails if the --master-host flag is missing", func() {
		checkSeginstallSession := runCommand("check", "seginstall")
		Expect(checkSeginstallSession).Should(Exit(1))
		Expect(string(checkSeginstallSession.Out.Contents())).To(Equal("Required flag(s) \"master-host\" have/has not been set\n"))
	})
})
