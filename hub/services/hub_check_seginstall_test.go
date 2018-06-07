package services_test

import (
	_ "github.com/greenplum-db/gpupgrade/hub/services"

	"github.com/greenplum-db/gpupgrade/hub/services"
	pb "github.com/greenplum-db/gpupgrade/idl"

	"io/ioutil"

	"github.com/greenplum-db/gpupgrade/testutils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
)

var _ = Describe("PrepareSeginstall", func() {

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
		hub = services.NewHub(testutils.CreateSampleClusterPair(), grpc.DialContext, commandExecer.Exec, conf, stubRemoteExecutor)
	})

	Describe("CheckSeginstall", func() {
		It("returns a gRPC reply object, if the software verification gets underway asynch", func() {
			_, err := hub.CheckSeginstall(nil, &pb.CheckSeginstallRequest{})
			Expect(err).ToNot(HaveOccurred())
			Eventually(stubRemoteExecutor.VerifySoftwareHosts).Should(Receive(Equal([]string{"hostone"})))
		})
	})
})
