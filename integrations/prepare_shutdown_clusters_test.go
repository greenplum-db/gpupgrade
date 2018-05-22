package integrations_test

import (
	"io/ioutil"
	"os"
	"strings"

	"github.com/greenplum-db/gpupgrade/hub/configutils"
	"github.com/greenplum-db/gpupgrade/hub/services"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"github.com/greenplum-db/gpupgrade/hub/cluster"
	"fmt"
)

var _ = Describe("prepare shutdown-clusters", func() {
	var (
		dir           string
		hub           *services.HubClient
		mockAgent     *testutils.MockAgentServer
		commandExecer *testutils.FakeCommandExecer
		outChan       chan []byte
		errChan       chan error
		oldBinDir     string
		newBinDir     string
	)

	BeforeEach(func() {
		var err error
		dir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		oldBinDir = "/old/tmp"
		newBinDir = "/new/tmp"

		config := `{"SegConfig":[{
			  "datadir": "/some/data/dir",
			  "content": -1,
			  "dbid": 1,
			  "hostname": "localhost",
			  "port": 5432
			}],"BinDir":"%s"}`

		testutils.WriteOldConfig(dir, fmt.Sprintf(config, oldBinDir))
		testutils.WriteNewConfig(dir, fmt.Sprintf(config, newBinDir))

		port, err = testutils.GetOpenPort()
		Expect(err).ToNot(HaveOccurred())

		var agentPort int
		mockAgent, agentPort = testutils.NewMockAgentServer()

		conf := &services.HubConfig{
			CliToHubPort:   port,
			HubToAgentPort: agentPort,
			StateDir:       dir,
		}
		reader := configutils.NewReader()

		outChan = make(chan []byte, 5)
		errChan = make(chan error, 5)

		commandExecer = &testutils.FakeCommandExecer{}
		commandExecer.SetOutput(&testutils.FakeCommand{
			Out: outChan,
			Err: errChan,
		})
		clusterPair := cluster.NewClusterPair(dir, commandExecer.Exec)

		clusterPair.OldMasterPort = 25437
		clusterPair.NewMasterPort = 35437
		clusterPair.OldMasterDataDirectory = "/old/datadir"
		clusterPair.NewMasterDataDirectory = "/new/datadir"

		hub = services.NewHub(clusterPair, &reader, grpc.DialContext, commandExecer.Exec, conf)
		go hub.Start()
	})

	AfterEach(func() {
		hub.Stop()
		mockAgent.Stop()
		os.RemoveAll(dir)
	})

	It("updates status PENDING and then to COMPLETE if successful", func(done Done) {
		defer close(done)
		mockAgent.StatusConversionResponse = &pb.CheckConversionStatusReply{
			Statuses: []string{},
		}

		Expect(runStatusUpgrade()).To(ContainSubstring("PENDING - Shutdown clusters"))

		commandExecer.SetOutput(&testutils.FakeCommand{
			Out: outChan,
			Err: errChan,
		})
		outChan <- []byte("pid1")

		prepareShutdownClustersSession := runCommand("prepare", "shutdown-clusters", "--old-bindir", oldBinDir, "--new-bindir", newBinDir)
		Eventually(prepareShutdownClustersSession).Should(Exit(0))

		allCalls := strings.Join(commandExecer.Calls(), "")
		Expect(allCalls).To(ContainSubstring(oldBinDir + "/gpstop -a"))
		Expect(allCalls).To(ContainSubstring(newBinDir + "/gpstop -a"))
		Eventually(runStatusUpgrade).Should(ContainSubstring("COMPLETE - Shutdown clusters"))
	})

	It("updates status to FAILED if it fails to run", func() {
		mockAgent.StatusConversionResponse = &pb.CheckConversionStatusReply{
			Statuses: []string{},
		}

		commandExecer.SetOutput(&testutils.FakeCommand{
			Out: outChan,
			Err: errChan,
		})
		Expect(runStatusUpgrade()).To(ContainSubstring("PENDING - Shutdown clusters"))

		errChan <- nil
		errChan <- nil
		errChan <- errors.New("start failed")

		prepareShutdownClustersSession := runCommand("prepare", "shutdown-clusters", "--old-bindir", oldBinDir, "--new-bindir", newBinDir)
		Eventually(prepareShutdownClustersSession).Should(Exit(0))

		allCalls := strings.Join(commandExecer.Calls(), "")
		Expect(allCalls).To(ContainSubstring(oldBinDir + "/gpstop -a"))
		Expect(allCalls).To(ContainSubstring(newBinDir + "/gpstop -a"))
		Eventually(runStatusUpgrade).Should(ContainSubstring("FAILED - Shutdown clusters"))
	})

	It("fails if the --old-bindir or --new-bindir flags are missing", func() {
		prepareShutdownClustersSession := runCommand("prepare", "shutdown-clusters")
		Expect(prepareShutdownClustersSession).Should(Exit(1))
		Expect(string(prepareShutdownClustersSession.Out.Contents())).To(Equal("Required flag(s) \"new-bindir\", \"old-bindir\" have/has not been set\n"))
	})
})
