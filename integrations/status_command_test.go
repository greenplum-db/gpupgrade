package integrations_test

import (
	"io/ioutil"
	"os"

	agentServices "github.com/greenplum-db/gpupgrade/agent/services"
	"github.com/greenplum-db/gpupgrade/hub/cluster"
	"github.com/greenplum-db/gpupgrade/hub/configutils"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/testutils"

	"github.com/onsi/gomega/gbytes"
	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	"path/filepath"
)

var _ = Describe("status", func() {
	var (
		dir           string
		hub           *services.Hub
		agent         *agentServices.AgentServer
		commandExecer *testutils.FakeCommandExecer
		stubRemoteExecutor *testutils.StubRemoteExecutor
	)

	BeforeEach(func() {
		var err error
		dir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		agentPort, err := testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		agentConf := agentServices.AgentConfig{
			Port:     agentPort,
			StateDir: dir,
		}

		agentExecer := &testutils.FakeCommandExecer{}
		agentExecer.SetOutput(&testutils.FakeCommand{})

		agent = agentServices.NewAgentServer(agentExecer.Exec, agentConf)
		go agent.Start()

		port, err = testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		conf := &services.HubConfig{
			CliToHubPort:   port,
			HubToAgentPort: agentPort,
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
		agent.Stop()
		os.RemoveAll(dir)
		Expect(checkPortIsAvailable(port)).To(BeTrue())
	})

	Describe("conversion", func() {
		It("Displays status information for all segments", func() {
			config := `{"SegConfig":[{
  			  "content": 2,
  			  "dbid": 7,
  			  "hostname": "localhost"
  			},
  			{
  			  "content": -1,
  			  "dbid": 1,
  			  "hostname": "localhost"
  			}],"BinDir":"/tmp"}`

			testutils.WriteOldConfig(dir, config)

			pathToSegUpgrade := filepath.Join(dir, "pg_upgrade", "seg-2")
			err := os.MkdirAll(pathToSegUpgrade, 0700)
			Expect(err).ToNot(HaveOccurred())

			f, err := os.Create(filepath.Join(pathToSegUpgrade, "1.done"))
			Expect(err).ToNot(HaveOccurred())
			f.WriteString("Upgrade complete\n")
			f.Close()

			statusSession := runCommand("status", "conversion")
			Eventually(statusSession).Should(Exit(0))

			Eventually(statusSession).Should(gbytes.Say("PENDING - DBID 1 - CONTENT ID -1 - MASTER - .+"))
			Eventually(statusSession).Should(gbytes.Say("COMPLETE - DBID 7 - CONTENT ID 2 - PRIMARY - .+"))
		})
	})

	Describe("upgrade", func() {
		It("Reports some demo status from the hub", func() {
			statusSession := runCommand("status", "upgrade")
			Eventually(statusSession).Should(Exit(0))

			Eventually(statusSession).Should(gbytes.Say("PENDING - Configuration Check"))
			Eventually(statusSession).Should(gbytes.Say("PENDING - Install binaries on segments"))
		})
	})
})
