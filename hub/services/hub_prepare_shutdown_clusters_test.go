package services_test

import (
	"io/ioutil"
	"os"

	"github.com/greenplum-db/gpupgrade/helpers"
	"github.com/greenplum-db/gpupgrade/hub/configutils"
	"github.com/greenplum-db/gpupgrade/hub/services"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"google.golang.org/grpc"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/greenplum-db/gpupgrade/testutils"
)

var _ = Describe("PrepareShutdownClusters", func() {
	var (
		reader  configutils.Reader
		conf    *services.HubConfig
		testLog *gbytes.Buffer
		stubRemoteExecutor *testutils.StubRemoteExecutor
	)
	BeforeEach(func() {
		_, _, testLog = testhelper.SetupTestLogger()
		utils.System.RemoveAll = func(s string) error { return nil }
		utils.System.MkdirAll = func(s string, perm os.FileMode) error { return nil }

		reader = configutils.NewReader()
		dir, err := ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
		conf = &services.HubConfig{
			StateDir: dir,
		}
		stubRemoteExecutor = testutils.NewStubRemoteExecutor()
	})

	AfterEach(func() {
		utils.InitializeSystemFunctions()
	})

	// ignoring the go routine
	It("returns successfully ", func() {

		clusterPair := &mockClusterPair{
			RunningPostmaster: true,
		}
		hub := services.NewHub(clusterPair, &reader, grpc.DialContext, nil, conf, stubRemoteExecutor)

		_, err := hub.PrepareShutdownClusters(nil, &pb.PrepareShutdownClustersRequest{})
		Expect(err).To(BeNil())
	})

	It("logs message if EitherPostmasterRunning returns false", func() {
		hub := services.NewHub(&mockClusterPair{}, &reader, grpc.DialContext, nil, conf, stubRemoteExecutor)

		_, err := hub.PrepareShutdownClusters(nil, &pb.PrepareShutdownClustersRequest{})
		Expect(err).To(BeNil())
		Expect(testLog.Contents()).To(ContainSubstring("PrepareShutdownClusters: neither postmaster was running, nothing to do"))
	})
})

type mockClusterPair struct {
	InitErr           error
	RunningPostmaster bool
}

func (c *mockClusterPair) StopEverything(str string) {}
func (c *mockClusterPair) Init(baseDir, oldPath, newPath string, execer helpers.CommandExecer) error {
	return c.InitErr
}
func (c *mockClusterPair) GetPortsAndDataDirForReconfiguration() (int, int, string) { return -1, -1, "" }
func (c *mockClusterPair) EitherPostmasterRunning() bool {
	return c.RunningPostmaster
}
