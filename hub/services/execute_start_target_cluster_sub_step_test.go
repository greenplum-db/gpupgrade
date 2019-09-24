package services

import (
	"os"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"

	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

)

var _ = Describe("upgrade validate start cluster", func() {
	var ctrl       *gomock.Controller
	var mockStream *mock_idl.MockCliToHub_ExecuteServer
	var target     *utils.Cluster

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockStream = mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		cluster := cluster.NewCluster([]cluster.SegConfig{cluster.SegConfig{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "basedir/seg-1"}})
		target = &utils.Cluster{
			Cluster:    cluster,
			BinDir:     "/target/bindir",
			ConfigPath: "my/config/path",
			Version:    dbconn.GPDBVersion{},
		}
		utils.System.RemoveAll = func(s string) error { return nil }
		utils.System.MkdirAll = func(s string, perm os.FileMode) error { return nil }

		mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)

		mockStream.EXPECT().
			Send(gomock.Any()).
			AnyTimes()

		execCommandIsPostmasterRunning = nil
		execCommandStopCluster = nil
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	It("successfully starts the target cluster", func() {
		execCommand = exectest.NewCommandWithVerifier(EmptyMain,
			func(path string, args ...string) {
				Expect(path).To(Equal("bash"))
				Expect(args).To(Equal([]string{"-c", "source /target/bindir/../greenplum_path.sh && /target/bindir/gpstart -a -d basedir/seg-1"}))
			})

		err := startNewCluster(mockStream, target)
		Expect(err).ToNot(HaveOccurred())
	})

	It("returns an error when it fails to start the target cluster", func() {
		execCommand = exectest.NewCommand(FailedMain)

		err := startNewCluster(mockStream, target)
		Expect(err.Error()).To(Equal("exit status 1"))
	})

})
