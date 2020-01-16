package hub

import (
	"testing"

	"github.com/golang/mock/gomock"
	"google.golang.org/grpc"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/gomega"
)

func TestCopyMaster(t *testing.T) {
	g := NewGomegaWithT(t)

	testhelper.SetupTestLogger() // initialize gplog

	sourceNodes := cluster.NewCluster([]cluster.SegConfig{
		cluster.SegConfig{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1"},
		cluster.SegConfig{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1"},
		cluster.SegConfig{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2"},
	})
	sourceCluster := utils.Cluster{
		Cluster: sourceNodes,
		BinDir:  "/source/bindir",
		Version: dbconn.GPDBVersion{},
	}

	targetNodes := cluster.NewCluster([]cluster.SegConfig{
		cluster.SegConfig{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1"},
		cluster.SegConfig{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1"},
		cluster.SegConfig{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2"},
	})
	targetCluster := utils.Cluster{
		Cluster: targetNodes,
		BinDir:  "/target/bindir",
		Version: dbconn.GPDBVersion{},
	}

	conf := &Config{
		Source:      &sourceCluster,
		Target:      &targetCluster,
		UseLinkMode: false,
	}
	hub := New(conf, grpc.DialContext, "", &upgradestatus.ChecklistManager{})

	t.Run("copies the master data directory to each primary host", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		testExecutor := &testhelper.TestExecutor{}
		mockOutput := &cluster.RemoteOutput{}
		testExecutor.ClusterOutput = mockOutput
		sourceNodes.Executor = testExecutor

		client := mock_idl.NewMockAgentClient(ctrl)
		client.EXPECT().CopyMaster(
			gomock.Any(),
			gomock.Any(),
		).Return(&idl.CopyMasterReply{}, nil)

		agentConns := []*Connection{
			{nil, client, "host1", nil},
		}

		hub.agentConns = agentConns

		err := hub.CopyMasterDataDir(DevNull)
		g.Expect(err).ToNot(HaveOccurred())

		g.Eventually(func() int { return testExecutor.NumExecutions }).Should(Equal(1))

		expectedCmds := []map[int][]string{{
			0: {"rsync", "-rzpogt", "/data/qddir/seg-1/", "host1:/tmp/masterDirCopy"},
			1: {"rsync", "-rzpogt", "/data/qddir/seg-1/", "host2:/tmp/masterDirCopy"},
		}}
		g.Expect(testExecutor.ClusterCommands).To(Equal(expectedCmds))
	})

	t.Run("copies the master data directory only once per host", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		testExecutor := &testhelper.TestExecutor{}
		mockOutput := &cluster.RemoteOutput{}
		testExecutor.ClusterOutput = mockOutput
		sourceNodes.Executor = testExecutor

		client := mock_idl.NewMockAgentClient(ctrl)
		client.EXPECT().CopyMaster(
			gomock.Any(),
			gomock.Any(),
		).Return(&idl.CopyMasterReply{}, nil)

		agentConns := []*Connection{
			{nil, client, "localhost", nil},
		}

		hub.agentConns = agentConns

		// Set all target segment hosts to be the same.
		for content, segment := range targetCluster.Segments {
			segment.Hostname = targetCluster.Segments[-1].Hostname
			targetCluster.Segments[content] = segment
		}

		err := hub.CopyMasterDataDir(DevNull)
		g.Expect(err).ToNot(HaveOccurred())

		g.Eventually(func() int { return testExecutor.NumExecutions }).Should(Equal(1))

		expectedCmd := testExecutor.ClusterCommands[0]
		g.Expect(expectedCmd).To(HaveLen(1))
		g.Expect(expectedCmd).To(ContainElement([]string{"rsync", "-rzpogt", "/data/qddir/seg-1/", "localhost:/tmp/masterDirCopy"}))
	})
}
