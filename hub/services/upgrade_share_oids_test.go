package services_test

import (
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/idl"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("UpgradeShareOids", func() {
	var (
		mockOutput   *cluster.RemoteOutput
		testExecutor *testhelper.TestExecutor
	)

	BeforeEach(func() {
		testExecutor = &testhelper.TestExecutor{}
		mockOutput = &cluster.RemoteOutput{}
		testExecutor.ClusterOutput = mockOutput
		source.Executor = testExecutor
	})

	It("copies the master data directory to each primary host in 6.0 or later", func() {
		source.Version = dbconn.NewVersion("6.0.0")
		_, err := hub.UpgradeShareOids(nil, &idl.UpgradeShareOidsRequest{})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() int { return testExecutor.NumExecutions }).Should(Equal(1))

		masterDataDir := filepath.Clean(source.MasterDataDir()) + string(filepath.Separator)
		resultMap := testExecutor.ClusterCommands[0]
		Expect(resultMap[0]).To(Equal([]string{"rsync", "-rzpogt", masterDataDir, "host1:/tmp/masterDirCopy"}))
		Expect(resultMap[1]).To(Equal([]string{"rsync", "-rzpogt", masterDataDir, "host2:/tmp/masterDirCopy"}))
	})

	It("copies the master data directory only once per host", func() {
		target.Version = dbconn.NewVersion("6.0.0")

		// Set all target segment hosts to be the same.
		for content, segment := range target.Segments {
			segment.Hostname = target.Segments[-1].Hostname
			target.Segments[content] = segment
		}

		_, err := hub.UpgradeShareOids(nil, &idl.UpgradeShareOidsRequest{})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() int { return testExecutor.NumExecutions }).Should(Equal(1))

		masterDataDir := filepath.Clean(source.MasterDataDir()) + string(filepath.Separator)
		resultMap := testExecutor.ClusterCommands[0]
		Expect(resultMap).To(HaveLen(1))
		Expect(resultMap).To(ContainElement([]string{"rsync", "-rzpogt", masterDataDir, "localhost:/tmp/masterDirCopy"}))
	})
})
