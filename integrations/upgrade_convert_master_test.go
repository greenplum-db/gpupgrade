package integrations_test

import (
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	"google.golang.org/grpc"
)

var _ = Describe("upgrade convert master", func() {
	var (
		hub *services.Hub
		cm  *testutils.MockChecklistManager
	)

	BeforeEach(func() {
		port, err := testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		conf := &services.HubConfig{
			CliToHubPort:   port,
			HubToAgentPort: 0,
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

	/*
	 * We don't have any integration tests testing the actual behavior of convert
	 * master because that function just performs setup and then calls pg_upgrade,
	 * so the setup logic can be tested in unit tests and pg_upgrade behavior will
	 * be tested in end-to-end tests.
	 *
	 * TODO: Add end-to-end tests for convert master
	 */

	It("fails if the --old-bindir or --new-bindir flags are missing", func() {
		prepareShutdownClustersSession := runCommand("upgrade", "convert-master")
		Expect(prepareShutdownClustersSession).Should(Exit(1))
		Expect(string(prepareShutdownClustersSession.Out.Contents())).To(Equal("Required flag(s) \"new-bindir\", \"new-datadir\", \"old-bindir\", \"old-datadir\" have/has not been set\n"))
	})
})
