package services

import (
	"bytes"
	"os"
	"os/exec"
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"

	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func StartClusterCmd()        {}
func StopClusterCmd()         {}
func IsPostmasterRunningCmd() {}
func IsPostmasterRunningCmd_Errors() {
	os.Stderr.WriteString("exit status 2")
	os.Exit(2)
}

func init() {
	exectest.RegisterMains(
		StartClusterCmd,
		StopClusterCmd,
		IsPostmasterRunningCmd,
		IsPostmasterRunningCmd_Errors,
	)
}

func TestStartOrStopCluster(t *testing.T) {
	g := NewGomegaWithT(t)
	ctrl := gomock.NewController(GinkgoT())
	defer ctrl.Finish()

	mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
	mockStream.EXPECT().
		Send(gomock.Any()).
		AnyTimes()

	var buf bytes.Buffer
	var source *utils.Cluster
	cluster := cluster.NewCluster([]cluster.SegConfig{cluster.SegConfig{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "basedir/seg-1"}})
	source = &utils.Cluster{
		Cluster:    cluster,
		BinDir:     "/source/bindir",
		ConfigPath: "my/config/path",
		Version:    dbconn.GPDBVersion{},
	}
	utils.System.RemoveAll = func(s string) error { return nil }
	utils.System.MkdirAll = func(s string, perm os.FileMode) error { return nil }

	startStopClusterCmd = nil
	isPostmasterRunningCmd = nil

	defer func() {
		startStopClusterCmd = exec.Command
		isPostmasterRunningCmd = exec.Command
	}()

	t.Run("isPostmasterRunning succeeds", func(t *testing.T) {
		isPostmasterRunningCmd = exectest.NewCommandWithVerifier(IsPostmasterRunningCmd,
			func(path string, args ...string) {
				g.Expect(path).To(Equal("bash"))
				g.Expect(args).To(Equal([]string{"-c", "pgrep -F basedir/seg-1/postmaster.pid"}))
			})

		err := IsPostmasterRunning(mockStream, &buf, source)
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("isPostmasterRunning fails", func(t *testing.T) {
		isPostmasterRunningCmd = exectest.NewCommand(IsPostmasterRunningCmd_Errors)

		err := IsPostmasterRunning(mockStream, &buf, source)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("stopCluster successfully shuts down cluster", func(t *testing.T) {
		isPostmasterRunningCmd = exectest.NewCommandWithVerifier(IsPostmasterRunningCmd,
			func(path string, args ...string) {
				g.Expect(path).To(Equal("bash"))
				g.Expect(args).To(Equal([]string{"-c", "pgrep -F basedir/seg-1/postmaster.pid"}))
			})

		startStopClusterCmd = exectest.NewCommandWithVerifier(StopClusterCmd,
			func(path string, args ...string) {
				g.Expect(path).To(Equal("bash"))
				g.Expect(args).To(Equal([]string{"-c", "source /source/bindir/../greenplum_path.sh " +
					"&& /source/bindir/gpstop -a -d basedir/seg-1"}))
			})

		err := StopCluster(mockStream, &buf, source)
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("stopCluster detects that cluster is already shutdown", func(t *testing.T) {
		isPostmasterRunningCmd = exectest.NewCommand(IsPostmasterRunningCmd_Errors)

		var skippedStopClusterCommand = true
		startStopClusterCmd = exectest.NewCommandWithVerifier(IsPostmasterRunningCmd,
			func(path string, args ...string) {
				skippedStopClusterCommand = false
			})

		err := StopCluster(mockStream, &buf, source)
		g.Expect(err).To(HaveOccurred())
		g.Expect(skippedStopClusterCommand).To(Equal(true))
	})

	t.Run("startCluster successfully starts up cluster", func(t *testing.T) {
		startStopClusterCmd = exectest.NewCommandWithVerifier(StartClusterCmd,
			func(path string, args ...string) {
				g.Expect(path).To(Equal("bash"))
				g.Expect(args).To(Equal([]string{"-c", "source /source/bindir/../greenplum_path.sh " +
					"&& /source/bindir/gpstart -a -d basedir/seg-1"}))
			})

		err := StartCluster(mockStream, &buf, source)
		g.Expect(err).ToNot(HaveOccurred())
	})
}
