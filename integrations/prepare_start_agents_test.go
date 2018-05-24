package integrations_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/greenplum-db/gpupgrade/hub/cluster"
	"github.com/greenplum-db/gpupgrade/hub/configutils"
	"github.com/greenplum-db/gpupgrade/hub/services"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	"google.golang.org/grpc"
	"github.com/greenplum-db/gpupgrade/hub/cluster_ssher"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"time"
)

var _ = Describe("prepare", func() {
	var (
		dir           string
		hub           *services.Hub
		mockAgent     *testutils.MockAgentServer
		commandExecer *testutils.FakeCommandExecer
	)

	BeforeEach(func() {
		var err error
		dir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		var agentPort int
		mockAgent, agentPort = testutils.NewMockAgentServer()

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

		clusterSsher := cluster_ssher.NewClusterSsher(
			upgradestatus.NewChecklistManager(conf.StateDir),
			services.NewPingerManager(conf.StateDir, 500*time.Millisecond),
			commandExecer.Exec,
		)
		hub = services.NewHub(&cluster.Pair{}, &reader, grpc.DialContext, commandExecer.Exec, conf, clusterSsher)

		pgPort := os.Getenv("PGPORT")

		clusterConfig := fmt.Sprintf(`{"SegConfig":[{
              "content": -1,
              "dbid": 1,
              "hostname": "localhost",
              "datadir": "%s",
              "mode": "s",
              "preferred_role": "m",
              "role": "m",
              "status": "u",
              "port": %s
        }],"BinDir":"/tmp"}`, dir, pgPort)

		testutils.WriteOldConfig(dir, clusterConfig)
		go hub.Start()
	})

	AfterEach(func() {
		hub.Stop()
		mockAgent.Stop()
		os.RemoveAll(dir)
		Expect(checkPortIsAvailable(port)).To(BeTrue())
	})

	Describe("start-agents", func() {
		It("updates status PENDING to RUNNING then to COMPLETE if successful", func(done Done) {
			defer close(done)

			mockAgent.StatusConversionResponse = &pb.CheckConversionStatusReply{
				Statuses: []string{},
			}

			Expect(runStatusUpgrade()).To(ContainSubstring("PENDING - Agents Started on Cluster"))

			trigger := make(chan struct{}, 1)
			commandExecer.SetTrigger(trigger)

			wg := &sync.WaitGroup{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer GinkgoRecover()

				Eventually(runStatusUpgrade).Should(ContainSubstring("RUNNING - Agents Started on Cluster"))
				trigger <- struct{}{}
			}()

			prepareStartAgentsSession := runCommand("prepare", "start-agents")
			Eventually(prepareStartAgentsSession).Should(Exit(0))
			wg.Wait()

			Expect(commandExecer.Command()).To(Equal("ssh"))
			Expect(strings.Join(commandExecer.Args(), "")).To(ContainSubstring("nohup"))
			Eventually(runStatusUpgrade).Should(ContainSubstring("COMPLETE - Agents Started on Cluster"))
		})
	})
})
