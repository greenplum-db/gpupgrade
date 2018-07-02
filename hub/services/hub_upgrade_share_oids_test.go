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

var _ = Describe("UpgradeShareOids", func() {
	var (
		hub          *services.Hub
		dir          string
		clusterPair  *utils.ClusterPair
		testExecutor *testhelper.TestExecutor
		cm           *testutils.MockChecklistManager
	)

	BeforeEach(func() {
		clusterPair = testutils.CreateSampleClusterPair()
		clusterPair.OldCluster.Segments[1] = cluster.SegConfig{Hostname: "hosttwo"}

		var err error
		dir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		hubConfig := &services.HubConfig{
			StateDir: dir,
		}
		testExecutor = &testhelper.TestExecutor{}
		clusterPair.OldCluster.Executor = testExecutor
		cm = testutils.NewMockChecklistManager()
		hub = services.NewHub(clusterPair, grpc.DialContext, hubConfig, cm)
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		os.RemoveAll(dir)
	})

	It("copies files to each host", func() {
		_, err := hub.UpgradeShareOids(nil, &pb.UpgradeShareOidsRequest{})
		Expect(err).ToNot(HaveOccurred())

		hostnames := clusterPair.GetHostnames()
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() int { return testExecutor.NumExecutions }).Should(Equal(len(hostnames)))

		Expect(testExecutor.LocalCommands).To(ConsistOf([]string{
			fmt.Sprintf("rsync -rzpogt %s/pg_upgrade/pg_upgrade_dump_*_oids.sql gpadmin@hostone:%s/pg_upgrade", dir, dir),
			fmt.Sprintf("rsync -rzpogt %s/pg_upgrade/pg_upgrade_dump_*_oids.sql gpadmin@hosttwo:%s/pg_upgrade", dir, dir),
		}))
	})

	It("copies all files even if rsync fails for a host", func() {
		testExecutor.LocalError = errors.New("failure")

		_, err := hub.UpgradeShareOids(nil, &pb.UpgradeShareOidsRequest{})
		Expect(err).ToNot(HaveOccurred())

		hostnames := clusterPair.GetHostnames()
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() int { return testExecutor.NumExecutions }).Should(Equal(len(hostnames)))
	})
})
