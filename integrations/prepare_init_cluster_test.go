package integrations_test

import (
	"io/ioutil"
	"os"

	"github.com/greenplum-db/gpupgrade/hub/cluster"
	"github.com/greenplum-db/gpupgrade/hub/configutils"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/testutils"

	"github.com/onsi/gomega/gbytes"
	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

// the `prepare start-hub` tests are currently in master_only_integration_test
var _ = Describe("prepare", func() {
	var (
		dir           string
		hub           *services.HubClient
		commandExecer *testutils.FakeCommandExecer
	)

	BeforeEach(func() {
		var err error
		dir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		port, err = testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		conf := &services.HubConfig{
			CliToHubPort:   port,
			HubToAgentPort: 6416,
			StateDir:       dir,
		}
		reader := configutils.NewReader()

		commandExecer = &testutils.FakeCommandExecer{}
		commandExecer.SetOutput(&testutils.FakeCommand{})

		hub = services.NewHub(&cluster.Pair{}, &reader, grpc.DialContext, commandExecer.Exec, conf)
		go hub.Start()
	})

	AfterEach(func() {
		hub.Stop()
		os.RemoveAll(dir)
		Expect(checkPortIsAvailable(port)).To(BeTrue())
	})

	/* This is demonstrating the limited implementation of init-cluster.
	    Assuming the user has already set up their new cluster, they should `init-cluster`
		with the port at which they stood it up, so the upgrade tool can create new_cluster_config

		In the future, the upgrade tool might take responsibility for starting its own cluster,
		in which case it won't need the port, but would still generate new_cluster_config
	*/
	Describe("Given that a gpdb cluster is up, in this case reusing the single cluster for other test.", func() {
		It("can save the database configuration json under the name 'new cluster'", func() {
			statusSessionPending := runCommand("status", "upgrade")
			Eventually(statusSessionPending).Should(gbytes.Say("PENDING - Initialize upgrade target cluster"))

			port := os.Getenv("PGPORT")
			Expect(port).ToNot(BeEmpty())

			session := runCommand("prepare", "init-cluster", "--port", port, "--new-bindir", "/tmp")
			Eventually(session).Should(Exit(0))

			Expect(runStatusUpgrade()).To(ContainSubstring("COMPLETE - Initialize upgrade target cluster"))

			reader := configutils.NewReader()
			reader.OfNewClusterConfig(dir)
			err := reader.Read()
			Expect(err).ToNot(HaveOccurred())

			Expect(len(reader.GetSegmentConfiguration())).To(BeNumerically(">", 1))
		})
	})

	It("fails if the port flag is missing", func() {
		prepareStartAgentsSession := runCommand("prepare", "init-cluster")
		Expect(prepareStartAgentsSession).Should(Exit(1))
		Expect(string(prepareStartAgentsSession.Out.Contents())).To(Equal("Required flag(s) \"new-bindir\", \"port\" have/has not been set\n"))
	})
})
