package integrations_test

import (
	"os"

	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	"google.golang.org/grpc"
)

// the `prepare start-hub` tests are currently in master_only_integration_test
var _ = Describe("prepare", func() {
	var (
		hub *services.Hub
		cm  *testutils.MockChecklistManager
	)

	BeforeEach(func() {
		var err error
		port, err = testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		conf := &services.HubConfig{
			CliToHubPort:   port,
			HubToAgentPort: 6416,
			StateDir:       testStateDir,
		}
		cm = testutils.NewMockChecklistManager()
		hub = services.NewHub(testutils.InitClusterPairFromDB(), grpc.DialContext, conf, cm)
		go hub.Start()
	})

	AfterEach(func() {
		hub.Stop()
		Expect(checkPortIsAvailable(port)).To(BeTrue())
	})

	/* This is demonstrating the limited implementation of init-cluster.
	    Assuming the user has already set up their new cluster, they should `init-cluster`
		with the port at which they stood it up, so the upgrade tool can create new_cluster_config

		In the future, the upgrade tool might take responsibility for starting its own cluster,
		in which case it won't need the port, but would still generate new_cluster_config
	*/
	It("can save the database configuration json under the name 'new cluster'", func() {
		port := os.Getenv("PGPORT")
		Expect(port).ToNot(BeEmpty())

		Expect(cm.IsPending(upgradestatus.INIT_CLUSTER)).To(BeTrue())
		session := runCommand("prepare", "init-cluster", "--port", port, "--new-bindir", "/new/bin/dir")
		Eventually(session).Should(Exit(0))

		Expect(cm.IsComplete(upgradestatus.INIT_CLUSTER)).To(BeTrue())

		cp := &utils.ClusterPair{}
		err := cp.ReadNewConfig(testStateDir)
		Expect(err).ToNot(HaveOccurred())

		Expect(len(cp.NewCluster.Segments)).To(BeNumerically(">", 1))
	})

	It("fails if required flags are missing", func() {
		prepareStartAgentsSession := runCommand("prepare", "init-cluster")
		Expect(prepareStartAgentsSession).Should(Exit(1))
		Expect(string(prepareStartAgentsSession.Out.Contents())).To(Equal("Required flag(s) \"new-bindir\", \"port\" have/has not been set\n"))
	})
})
