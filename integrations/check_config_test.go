package integrations_test

import (
	"fmt"

	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	"google.golang.org/grpc"
)

// needs the cli and the hub
var _ = Describe("check config", func() {
	var (
		hub            *services.Hub
		hubToAgentPort int
	)

	BeforeEach(func() {
		hubToAgentPort = 6416

		var err error

		port, err = testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		conf := &services.HubConfig{
			CliToHubPort:   port,
			HubToAgentPort: hubToAgentPort,
			StateDir:       testStateDir,
		}

		cm := testutils.NewMockChecklistManager()
		hub = services.NewHub(testutils.InitClusterPairFromDB(), grpc.DialContext, conf, cm)
		go hub.Start()
	})

	AfterEach(func() {
		hub.Stop()
	})

	It("happy: the database configuration is saved to a specified location", func() {
		session := runCommand("check", "config", "--master-host", "localhost", "--old-bindir", "/old/bin/dir")
		if session.ExitCode() != 0 {
			fmt.Println("make sure greenplum is running")
		}
		Expect(session).To(Exit(0))

		cp := &utils.ClusterPair{}
		err := cp.ReadOldConfig(testStateDir)
		testutils.Check("cannot read config", err)

		Expect(len(cp.OldCluster.Segments)).To(BeNumerically(">", 1))
	})

	It("fails if required flags are missing", func() {
		checkConfigSession := runCommand("check", "config")
		Expect(checkConfigSession).Should(Exit(1))
		Expect(string(checkConfigSession.Out.Contents())).To(Equal("Required flag(s) \"master-host\", \"old-bindir\" have/has not been set\n"))
	})
})
