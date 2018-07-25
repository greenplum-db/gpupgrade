package services_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/hub/services"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"

	"google.golang.org/grpc"

	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("UpgradeReconfigurePorts", func() {
	var (
		hub          *services.Hub
		dir          string
		testExecutor *testhelper.TestExecutor
		cm           *testutils.MockChecklistManager
	)

	BeforeEach(func() {
		var err error
		dir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		numInvocations := 0
		utils.System.ReadFile = func(filename string) ([]byte, error) {
			if numInvocations == 0 {
				numInvocations++
				return []byte(testutils.MASTER_ONLY_JSON), nil
			} else {
				return []byte(testutils.NEW_MASTER_JSON), nil
			}
		}

		source, target := testutils.CreateSampleClusterPair()
		testExecutor = &testhelper.TestExecutor{}
		source.Segments[1] = cluster.SegConfig{Hostname: "hosttwo"}
		source.Executor = testExecutor
		hubConfig := &services.HubConfig{
			StateDir: dir,
		}
		cm = testutils.NewMockChecklistManager()
		hub = services.NewHub(source, target, grpc.DialContext, hubConfig, cm)
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		os.RemoveAll(dir)
	})

	It("reconfigures port in postgresql.conf on master", func() {
		reply, err := hub.UpgradeReconfigurePorts(nil, &pb.UpgradeReconfigurePortsRequest{})
		Expect(reply).To(Equal(&pb.UpgradeReconfigurePortsReply{}))
		Expect(err).To(BeNil())
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring(fmt.Sprintf(services.SedAndMvString, 35437, 25437, "/target/datadir")))
	})

	It("returns err if reconfigure cmd fails", func() {
		testExecutor.LocalError = errors.New("boom")
		reply, err := hub.UpgradeReconfigurePorts(nil, &pb.UpgradeReconfigurePortsRequest{})
		Expect(reply).To(BeNil())
		Expect(err).ToNot(BeNil())
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring(fmt.Sprintf(services.SedAndMvString, 35437, 25437, "/target/datadir")))
	})

})
