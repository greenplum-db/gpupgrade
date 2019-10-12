package services_test

import (
	"bytes"
	"path/filepath"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/idl/mock_idl"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ExecuteCopyMasterSubStep", func() {
	var (
		mockOutput   *cluster.RemoteOutput
		testExecutor *testhelper.TestExecutor

		ctrl       *gomock.Controller
		mockStream *mock_idl.MockCliToHub_ExecuteServer
		buf bytes.Buffer
	)

	BeforeEach(func() {
		testExecutor = &testhelper.TestExecutor{}
		mockOutput = &cluster.RemoteOutput{}
		testExecutor.ClusterOutput = mockOutput
		source.Executor = testExecutor

		ctrl = gomock.NewController(GinkgoT())
		mockStream = mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	It("copies the master data directory to each primary host in 6.0 or later", func() {
		mockStream.EXPECT().
			Send(gomock.Any()).
			AnyTimes()

		source.Version = dbconn.NewVersion("6.0.0")
		err := hub.CopyMasterDataDir(mockStream, &buf)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() int { return testExecutor.NumExecutions }).Should(Equal(1))

		masterDataDir := filepath.Clean(source.MasterDataDir()) + string(filepath.Separator)
		resultMap := testExecutor.ClusterCommands[0]
		Expect(resultMap[0]).To(Equal([]string{"rsync", "-rzpogt", masterDataDir, "host1:/tmp/masterDirCopy"}))
		Expect(resultMap[1]).To(Equal([]string{"rsync", "-rzpogt", masterDataDir, "host2:/tmp/masterDirCopy"}))
	})

	It("copies the master data directory only once per host", func() {
		mockStream.EXPECT().
			Send(gomock.Any()).
			AnyTimes()

		target.Version = dbconn.NewVersion("6.0.0")

		// Set all target segment hosts to be the same.
		for content, segment := range target.Segments {
			segment.Hostname = target.Segments[-1].Hostname
			target.Segments[content] = segment
		}

		err := hub.CopyMasterDataDir(mockStream, &buf)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() int { return testExecutor.NumExecutions }).Should(Equal(1))

		masterDataDir := filepath.Clean(source.MasterDataDir()) + string(filepath.Separator)
		resultMap := testExecutor.ClusterCommands[0]
		Expect(resultMap).To(HaveLen(1))
		Expect(resultMap).To(ContainElement([]string{"rsync", "-rzpogt", masterDataDir, "localhost:/tmp/masterDirCopy"}))
	})
})
