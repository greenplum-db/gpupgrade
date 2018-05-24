package integrations_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"time"

	"github.com/greenplum-db/gpupgrade/hub/cluster"
	"github.com/greenplum-db/gpupgrade/hub/configutils"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
	. "github.com/onsi/gomega/gexec"
	"google.golang.org/grpc"
)

var _ = Describe("version command", func() {
	var (
		dir           string
		hub           *services.Hub
		commandExecer *testutils.FakeCommandExecer
		stubRemoteExecutor *testutils.StubRemoteExecutor
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

		stubRemoteExecutor = testutils.NewStubRemoteExecutor()
		hub = services.NewHub(&cluster.Pair{}, &reader, grpc.DialContext, commandExecer.Exec, conf, stubRemoteExecutor)
		go hub.Start()
	})

	AfterEach(func() {
		hub.Stop()
		os.RemoveAll(dir)
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
