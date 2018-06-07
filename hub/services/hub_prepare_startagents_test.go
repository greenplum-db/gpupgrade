package services_test

import (
	_ "github.com/greenplum-db/gpupgrade/hub/services"

	"github.com/greenplum-db/gpupgrade/hub/services"

	"io/ioutil"

	"github.com/greenplum-db/gpupgrade/testutils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
)

var _ = Describe("PrepareStartAgents", func() {
	var (
		dir                string
		stubRemoteExecutor *testutils.StubRemoteExecutor
		commandExecer      *testutils.FakeCommandExecer
		outChan            chan []byte
		errChan            chan error
		hub                *services.Hub
	)

	BeforeEach(func() {
		var err error
		dir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
		conf := &services.HubConfig{
			StateDir: dir,
		}
		stubRemoteExecutor = testutils.NewStubRemoteExecutor()
		errChan = make(chan error, 2)
		outChan = make(chan []byte, 2)
		commandExecer = &testutils.FakeCommandExecer{}
		commandExecer.SetOutput(&testutils.FakeCommand{
			Err: errChan,
			Out: outChan,
		})
		clusterPair := testutils.CreateSampleClusterPair()
		hub = services.NewHub(clusterPair, grpc.DialContext, commandExecer.Exec, conf, stubRemoteExecutor)
	})

	Describe("PrepareStartAgents", func() {
		It("returns a gRPC object", func() {
			reply, err := hub.PrepareStartAgents(nil, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(reply).ToNot(BeNil())
			Eventually(stubRemoteExecutor.StartHosts).Should(Receive(Equal([]string{"hostone"})))
		})
	})

})
