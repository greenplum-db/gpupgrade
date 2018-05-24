package services_test

import (
	_ "github.com/greenplum-db/gpupgrade/hub/services"

	"github.com/greenplum-db/gpupgrade/hub/services"
	pb "github.com/greenplum-db/gpupgrade/idl"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/greenplum-db/gpupgrade/testutils"
	"google.golang.org/grpc"
	"io/ioutil"
)

var _ = Describe("PrepareSeginstall", func() {

	var (
		dir           string
		spyConfigReader    *testutils.SpyConfigReader
		stubRemoteExecutor *testutils.StubRemoteExecutor
		commandExecer *testutils.FakeCommandExecer
		outChan       chan []byte
		errChan       chan error
		hub           *services.Hub
	)

	BeforeEach(func() {
		var err error
		dir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
		conf := &services.HubConfig{
			StateDir: dir,
		}
		spyConfigReader = testutils.NewSpyConfigReader()
		stubRemoteExecutor = testutils.NewStubRemoteExecutor()
		errChan = make(chan error, 2)
		outChan = make(chan []byte, 2)
		commandExecer = &testutils.FakeCommandExecer{}
		commandExecer.SetOutput(&testutils.FakeCommand{
			Err: errChan,
			Out: outChan,
		})
		hub = services.NewHub(nil, spyConfigReader, grpc.DialContext, commandExecer.Exec, conf, stubRemoteExecutor)
	})

	Describe("CheckSeginstall", func() {
		It("returns a gRPC reply object, if the software verification gets underway asynch", func() {
			spyConfigReader.FailToGetHostnames = false

			_, err := hub.CheckSeginstall(nil, &pb.CheckSeginstallRequest{})
			Expect(err).ToNot(HaveOccurred())
			Eventually(stubRemoteExecutor.VerifySoftwareHosts).Should(Receive(Equal([]string{"somehost"})))
		})

		It("returns an error if cluster config can't be read", func() {
			spyConfigReader.FailToGetHostnames = true

			_, err := hub.CheckSeginstall(nil, &pb.CheckSeginstallRequest{})
			Expect(err).To(HaveOccurred())
		})

		It("returns an error if cluster config is empty", func() {
			spyConfigReader.FailToGetHostnames = false
			spyConfigReader.HostnamesListEmpty = true

			_, err := hub.CheckSeginstall(nil, &pb.CheckSeginstallRequest{})
			Expect(err).To(HaveOccurred())
		})
	})
})
