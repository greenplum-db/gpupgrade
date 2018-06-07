package services_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
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
		hub                *services.Hub
		dir                string
		commandExecer      *testutils.FakeCommandExecer
		errChan            chan error
		outChan            chan []byte
		stubRemoteExecutor *testutils.StubRemoteExecutor
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

		errChan = make(chan error, 2)
		outChan = make(chan []byte, 2)
		commandExecer = &testutils.FakeCommandExecer{}
		commandExecer.SetOutput(&testutils.FakeCommand{
			Err: errChan,
			Out: outChan,
		})
		clusterPair := testutils.CreateSampleClusterPair()
		clusterPair.OldCluster.Segments[1] = cluster.SegConfig{Hostname: "hosttwo"}
		stubRemoteExecutor = testutils.NewStubRemoteExecutor()
		hub = services.NewHub(clusterPair, grpc.DialContext, commandExecer.Exec, &services.HubConfig{
			StateDir: dir,
		}, stubRemoteExecutor)
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		os.RemoveAll(dir)
	})

	It("reconfigures port in postgresql.conf on master", func() {
		reply, err := hub.UpgradeReconfigurePorts(nil, &pb.UpgradeReconfigurePortsRequest{})
		Expect(reply).To(Equal(&pb.UpgradeReconfigurePortsReply{}))
		Expect(err).To(BeNil())
		Expect(commandExecer.Calls()[0]).To(ContainSubstring(fmt.Sprintf(services.SedAndMvString, 35437, 25437, "/new/datadir")))
	})

	It("returns err if reconfigure cmd fails", func() {
		errChan <- errors.New("boom")
		reply, err := hub.UpgradeReconfigurePorts(nil, &pb.UpgradeReconfigurePortsRequest{})
		Expect(reply).To(BeNil())
		Expect(err).ToNot(BeNil())
		Expect(commandExecer.Calls()[0]).To(ContainSubstring(fmt.Sprintf(services.SedAndMvString, 35437, 25437, "/new/datadir")))
	})

})
