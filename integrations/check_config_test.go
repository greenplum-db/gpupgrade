package integrations_test

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/greenplum-db/gpupgrade/hub/cluster"
	"github.com/greenplum-db/gpupgrade/hub/configutils"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	"google.golang.org/grpc"
	"github.com/greenplum-db/gpupgrade/hub/cluster_ssher"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"time"
)

// needs the cli and the hub
var _ = Describe("check config", func() {
	var (
		dir            string
		hub            *services.Hub
		commandExecer  *testutils.FakeCommandExecer
		hubToAgentPort int
	)

	BeforeEach(func() {
		hubToAgentPort = 6416

		var err error
		dir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		// We only needed to get the name of the temp directory, so we delete it.
		// The actual directory will be created by the
		// SaveOldClusterConfigAndVersion() routine.
		// Being a temp dir, Go will remove the directory at the end of test also.
		err = os.RemoveAll(dir)
		Expect(err).ToNot(HaveOccurred())

		port, err = testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		conf := &services.HubConfig{
			CliToHubPort:   port,
			HubToAgentPort: hubToAgentPort,
			StateDir:       dir,
		}
		reader := configutils.NewReader()

		commandExecer = &testutils.FakeCommandExecer{}
		commandExecer.SetOutput(&testutils.FakeCommand{})

		clusterSsher := cluster_ssher.NewClusterSsher(
			upgradestatus.NewChecklistManager(conf.StateDir),
			services.NewPingerManager(conf.StateDir, 500*time.Millisecond),
			commandExecer.Exec,
		)
		hub = services.NewHub(&cluster.Pair{}, &reader, grpc.DialContext, commandExecer.Exec, conf, clusterSsher)
		go hub.Start()
	})

	AfterEach(func() {
		hub.Stop()
		os.RemoveAll(dir)
	})

	Describe("when a greenplum master db on localhost is up and running", func() {
		It("happy: the database configuration is saved to a specified location", func() {
			//testutils.WriteSampleConfigVersion(dir)
			session := runCommand("check", "config", "--master-host", "localhost", "--old-bindir", "/tmp")
			if session.ExitCode() != 0 {
				fmt.Println("make sure greenplum is running")
			}
			Expect(session).To(Exit(0))

			_, err := ioutil.ReadFile(configutils.GetConfigFilePath(dir))
			testutils.Check("cannot read file", err)

			reader := configutils.Reader{}
			reader.OfOldClusterConfig(dir)
			err = reader.Read()
			testutils.Check("cannot read config", err)

			Expect(len(reader.GetSegmentConfiguration())).To(BeNumerically(">", 1))
		})
	})

	It("fails if the --master-host flag is missing", func() {
		checkConfigSession := runCommand("check", "config")
		Expect(checkConfigSession).Should(Exit(1))
		Expect(string(checkConfigSession.Out.Contents())).To(Equal("Required flag(s) \"master-host\", \"old-bindir\" have/has not been set\n"))
	})
})
