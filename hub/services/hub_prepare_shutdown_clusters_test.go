package services_test

import (
	"errors"
	"io/ioutil"
	"os"

	"github.com/greenplum-db/gpupgrade/hub/services"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"google.golang.org/grpc"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/testutils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("PrepareShutdownClusters", func() {
	var (
		conf               *services.HubConfig
		testLog            *gbytes.Buffer
		stubRemoteExecutor *testutils.StubRemoteExecutor
		clusterPair        *services.ClusterPair
	)
	BeforeEach(func() {
		_, _, testLog = testhelper.SetupTestLogger()
		utils.System.RemoveAll = func(s string) error { return nil }
		utils.System.MkdirAll = func(s string, perm os.FileMode) error { return nil }

		dir, err := ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
		conf = &services.HubConfig{
			StateDir: dir,
		}
		stubRemoteExecutor = testutils.NewStubRemoteExecutor()
		clusterPair = testutils.CreateSampleClusterPair()
		clusterPair.OldCluster.Executor = &testhelper.TestExecutor{}
	})

	AfterEach(func() {
		utils.InitializeSystemFunctions()
	})

	// ignoring the go routine
	It("returns successfully ", func() {
		hub := services.NewHub(clusterPair, grpc.DialContext, nil, conf, stubRemoteExecutor)

		_, err := hub.PrepareShutdownClusters(nil, &pb.PrepareShutdownClustersRequest{})
		Expect(err).To(BeNil())
	})

	It("logs message if EitherPostmasterRunning returns false", func() {
		clusterPair.OldCluster.Executor = &testhelper.TestExecutor{
			LocalError: errors.New("generic error"),
		}
		hub := services.NewHub(clusterPair, grpc.DialContext, nil, conf, stubRemoteExecutor)

		_, err := hub.PrepareShutdownClusters(nil, &pb.PrepareShutdownClustersRequest{})
		Expect(err).To(BeNil())
		Expect(testLog.Contents()).To(ContainSubstring("PrepareShutdownClusters: neither postmaster was running, nothing to do"))
	})
})
