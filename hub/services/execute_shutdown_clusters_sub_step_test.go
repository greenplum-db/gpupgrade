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

func FailedMain() {
	os.Exit(1)
}

func init() {
	exectest.RegisterMains(
		EmptyMain,
		FailedMain,
	)
}

var _ = Describe("ExecuteShutdownClustersSubStep", func() {
	var ctrl       *gomock.Controller
	var mockStream *mock_idl.MockCliToHub_ExecuteServer
	var source     *utils.Cluster

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockStream = mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		cluster := cluster.NewCluster([]cluster.SegConfig{cluster.SegConfig{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "basedir/seg-1"}})
		source = &utils.Cluster{
			Cluster:    cluster,
			BinDir:     "/source/bindir",
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

	It("isPostmasterRunning() succeeds", func() {
		execCommandIsPostmasterRunning = exectest.NewCommandWithVerifier(EmptyMain,
			func(path string, args ...string) {
				Expect(path).To(Equal("bash"))
				Expect(args).To(Equal([]string{"-c", "pgrep -F basedir/seg-1/postmaster.pid"}))
			})

		err := IsPostmasterRunning(mockStream, source)
		Expect(err).ToNot(HaveOccurred())
	})

	It("isPostmasterRunning() fails", func() {
		execCommandIsPostmasterRunning = exectest.NewCommand(FailedMain)

		err := IsPostmasterRunning(mockStream, source)
		Expect(err).To(HaveOccurred())
	})

	It("stopCluster() successfully shuts down cluster", func() {
		execCommandIsPostmasterRunning = exectest.NewCommandWithVerifier(EmptyMain,
			func(path string, args ...string) {
				Expect(path).To(Equal("bash"))
				Expect(args).To(Equal([]string{"-c", "pgrep -F basedir/seg-1/postmaster.pid"}))
			})

		execCommandStopCluster = exectest.NewCommandWithVerifier(EmptyMain,
			func(path string, args ...string) {
				Expect(path).To(Equal("bash"))
				Expect(args).To(Equal([]string{"-c", "source /source/bindir/../greenplum_path.sh && /source/bindir/gpstop -a -d basedir/seg-1"}))
			})

		err := StopCluster(mockStream, source)
		Expect(err).ToNot(HaveOccurred())
	})

	It("stopCluster() detects that cluster is already shutdown", func() {
		execCommandIsPostmasterRunning = exectest.NewCommand(FailedMain)
		var skippedStopClusterCommand = true
		execCommandStopCluster = exectest.NewCommandWithVerifier(EmptyMain,
			func(path string, args ...string) {
				skippedStopClusterCommand = false
			})

		err := StopCluster(mockStream, source)
		Expect(err).To(HaveOccurred())
		Expect(skippedStopClusterCommand).To(Equal(true))
	})
})
