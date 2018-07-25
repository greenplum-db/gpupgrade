package services_test

import (
	"errors"
	"io/ioutil"
	"os"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"

	"google.golang.org/grpc"

	"github.com/greenplum-db/gpupgrade/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("upgrade validate start cluster", func() {
	var (
		hub          *services.Hub
		dir          string
		errChan      chan error
		outChan      chan []byte
		source       *utils.Cluster
		target       *utils.Cluster
		testExecutor *testhelper.TestExecutor
		cm           *testutils.MockChecklistManager
	)

	BeforeEach(func() {
		var err error
		dir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		errChan = make(chan error, 1)
		outChan = make(chan []byte, 1)

		source, target = testutils.CreateSampleClusterPair()
		testExecutor = &testhelper.TestExecutor{}
		target.Executor = testExecutor
		cm = testutils.NewMockChecklistManager()
		hub = services.NewHub(source, target, grpc.DialContext, &services.HubConfig{
			StateDir: dir,
		}, cm)
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		os.RemoveAll(dir)
	})

	It("sets status to COMPLETE when validate start cluster request has been made and returns no error", func() {
		Expect(cm.IsPending(upgradestatus.VALIDATE_START_CLUSTER)).To(BeTrue())

		_, err := hub.UpgradeValidateStartCluster(nil, &pb.UpgradeValidateStartClusterRequest{})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() bool { return cm.IsComplete(upgradestatus.VALIDATE_START_CLUSTER) }).Should(BeTrue())
		Expect(testExecutor.NumExecutions).To(Equal(1))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("source /target/bindir/../greenplum_path.sh"))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("/target/bindir/gpstart -a -d /target/datadir"))
	})

	It("sets status to FAILED when the validate start cluster request returns an error", func() {
		testExecutor.LocalError = errors.New("some error")

		_, err := hub.UpgradeValidateStartCluster(nil, &pb.UpgradeValidateStartClusterRequest{})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() bool { return cm.IsFailed(upgradestatus.VALIDATE_START_CLUSTER) }).Should(BeTrue())
	})
})
