package integrations_test

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/greenplum-db/gpupgrade/hub/cluster_ssher"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
	. "github.com/onsi/gomega/gexec"
	"google.golang.org/grpc"
)

var _ = Describe("version command", func() {
	var (
		hub           *services.Hub
		commandExecer *testutils.FakeCommandExecer
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
		commandExecer = &testutils.FakeCommandExecer{}
		commandExecer.SetOutput(&testutils.FakeCommand{})

		clusterSsher := cluster_ssher.NewClusterSsher(
			upgradestatus.NewChecklistManager(conf.StateDir),
			services.NewPingerManager(conf.StateDir, 500*time.Millisecond),
			commandExecer.Exec,
		)
		hub = services.NewHub(testutils.InitClusterPairFromDB(), grpc.DialContext, commandExecer.Exec, conf, clusterSsher)
		go hub.Start()
	})

	AfterEach(func() {
		hub.Stop()
		Expect(checkPortIsAvailable(port)).To(BeTrue())
	})

	It("reports the version that's injected at build-time", func() {
		fake_version := fmt.Sprintf("v0.0.0-dev.%d", time.Now().Unix())
		commandPathWithVersion, err := Build("github.com/greenplum-db/gpupgrade/cli", "-ldflags", "-X github.com/greenplum-db/gpupgrade/cli/commanders.UpgradeVersion="+fake_version)
		Expect(err).NotTo(HaveOccurred())

		// can't use the runCommand() integration helper function because we calculated a separate path
		cmd := exec.Command(commandPathWithVersion, "version")
		session, err := Start(cmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())

		Eventually(session).Should(Exit(0))
		Consistently(session.Out).ShouldNot(Say("unknown version"))
		Eventually(session.Out).Should(Say("gpupgrade version")) //scans session.Out buffer beyond the matching tokens
		Eventually(session.Out).Should(Say(fake_version))
	})
})
