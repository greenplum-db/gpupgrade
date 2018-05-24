package integrations_test

import (
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/greenplum-db/gpupgrade/hub/cluster"
	"github.com/greenplum-db/gpupgrade/hub/configutils"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

var _ = Describe("upgrade validate-start-cluster", func() {
	var (
		dir           string
		hub           *services.Hub
		commandExecer *testutils.FakeCommandExecer
		outChan       chan []byte
		errChan       chan error
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

		outChan = make(chan []byte, 2)
		errChan = make(chan error, 2)

		commandExecer = &testutils.FakeCommandExecer{}
		commandExecer.SetOutput(&testutils.FakeCommand{
			Out: outChan,
			Err: errChan,
		})

		stubRemoteExecutor = testutils.NewStubRemoteExecutor()
		hub = services.NewHub(&cluster.Pair{}, &reader, grpc.DialContext, commandExecer.Exec, conf, stubRemoteExecutor)
		go hub.Start()
	})

	AfterEach(func() {
		hub.Stop()
		os.RemoveAll(dir)
		Expect(checkPortIsAvailable(port)).To(BeTrue())
	})

	It("updates status PENDING to RUNNING then to COMPLETE if successful", func(done Done) {
		defer close(done)
		newBinDir, err := ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
		newDataDir, err := ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		Expect(runStatusUpgrade()).To(ContainSubstring("PENDING - Validate the upgraded cluster can start up"))

		trigger := make(chan struct{}, 1)
		commandExecer.SetTrigger(trigger)

		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer GinkgoRecover()

			Eventually(runStatusUpgrade).Should(ContainSubstring("RUNNING - Validate the upgraded cluster can start up"))
			trigger <- struct{}{}
		}()

		session := runCommand("upgrade", "validate-start-cluster", "--new-bindir", newBinDir, "--new-datadir", newDataDir)
		Eventually(session).Should(Exit(0))
		wg.Wait()

		Expect(commandExecer.Command()).To(Equal("bash"))
		Expect(strings.Join(commandExecer.Args(), "")).To(ContainSubstring("gpstart"))
		Eventually(runStatusUpgrade).Should(ContainSubstring("COMPLETE - Validate the upgraded cluster can start up"))
	})

	It("updates status to FAILED if it fails to run", func() {
		newBinDir, err := ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
		newDataDir, err := ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		Expect(runStatusUpgrade()).To(ContainSubstring("PENDING - Validate the upgraded cluster can start up"))

		errChan <- errors.New("start failed")

		session := runCommand("upgrade", "validate-start-cluster", "--new-bindir", newBinDir, "--new-datadir", newDataDir)
		Eventually(session).Should(Exit(0))

		Expect(commandExecer.Command()).To(Equal("bash"))
		Expect(strings.Join(commandExecer.Args(), "")).To(ContainSubstring("gpstart"))
		Eventually(runStatusUpgrade).Should(ContainSubstring("FAILED - Validate the upgraded cluster can start up"))
	})

	It("fails if the --new-bindir or --new-datadir flags are missing", func() {
		session := runCommand("upgrade", "validate-start-cluster")
		Expect(session).Should(Exit(1))
		Expect(string(session.Out.Contents())).To(Equal("Required flag(s) \"new-bindir\", \"new-datadir\" have/has not been set\n"))
	})
})
