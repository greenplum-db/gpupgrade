package hub_test

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestRenameMasterDataDir(t *testing.T) {
	t.Run("renames source master data dir", func(t *testing.T) {
		utils.System.Rename = func(src, dst string) error {
			expectedSrc := "/data/qddir"
			if src != expectedSrc {
				t.Errorf("got %q want %q", src, expectedSrc)
			}

			expectedDst := "/data/qddir_old"
			if dst != expectedDst {
				t.Errorf("got %q want %q", dst, expectedDst)
			}

			return nil
		}

		err := hub.RenameMasterDataDir("/data/qddir/demoDataDir-1", true)
		if err != nil {
			t.Errorf("unexpected error got %#v", err)
		}
	})

	t.Run("renames target master data dir", func(t *testing.T) {
		utils.System.Rename = func(src, dst string) error {
			expectedSrc := "/data/qddir_upgrade"
			if src != expectedSrc {
				t.Errorf("got %q want %q", src, expectedSrc)
			}

			expectedDst := "/data/qddir"
			if dst != expectedDst {
				t.Errorf("got %q want %q", dst, expectedDst)
			}

			return nil
		}

		err := hub.RenameMasterDataDir("/data/qddir/demoDataDir-1", false)
		if err != nil {
			t.Errorf("unexpected error got %#v", err)
		}
	})

	t.Run("returns error when rename fails", func(t *testing.T) {
		expected := errors.New("permission denied")
		utils.System.Rename = func(src, dst string) error {
			return expected
		}

		err := hub.RenameMasterDataDir("/data/qddir/demoDataDir-1", true)
		if !xerrors.Is(err, expected) {
			t.Errorf("got %#v want %#v", err, expected)
		}
	})
}

func TestRenameSegmentDataDirs(t *testing.T) {
	c := hub.MustCreateCluster(t, []utils.SegConfig{
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: utils.PrimaryRole},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Role: utils.PrimaryRole},
		{ContentID: 2, DbID: 4, Port: 25434, Hostname: "sdw1", DataDir: "/data/dbfast1/seg3", Role: utils.PrimaryRole},
		{ContentID: 3, DbID: 5, Port: 25435, Hostname: "sdw2", DataDir: "/data/dbfast2/seg4", Role: utils.PrimaryRole},
	})

	testhelper.SetupTestLogger() // initialize gplog

	t.Run("transforms source directories", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client1 := mock_idl.NewMockAgentClient(ctrl)
		client1.EXPECT().RenameDirectories(
			gomock.Any(),
			&idl.RenameDirectoriesRequest{
				Pairs: []*idl.RenamePair{{
					Src: "/data/dbfast1_upgrade",
					Dst: "/data/dbfast1",
				}},
			},
		).Return(&idl.RenameDirectoriesReply{}, nil)

		client2 := mock_idl.NewMockAgentClient(ctrl)
		client2.EXPECT().RenameDirectories(
			gomock.Any(),
			&idl.RenameDirectoriesRequest{
				Pairs: []*idl.RenamePair{{
					Src: "/data/dbfast2_upgrade",
					Dst: "/data/dbfast2",
				}},
			},
		).Return(&idl.RenameDirectoriesReply{}, nil)

		agentConns := []*hub.Connection{
			{nil, client1, "sdw1", nil},
			{nil, client2, "sdw2", nil},
		}

		err := hub.RenameSegmentDataDirs(agentConns, c, hub.UpgradeSuffix, true)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("transforms destination directories", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client1 := mock_idl.NewMockAgentClient(ctrl)
		client1.EXPECT().RenameDirectories(
			gomock.Any(),
			&idl.RenameDirectoriesRequest{
				Pairs: []*idl.RenamePair{{
					Src: "/data/dbfast1",
					Dst: "/data/dbfast1_old",
				}},
			},
		).Return(&idl.RenameDirectoriesReply{}, nil)

		client2 := mock_idl.NewMockAgentClient(ctrl)
		client2.EXPECT().RenameDirectories(
			gomock.Any(),
			&idl.RenameDirectoriesRequest{
				Pairs: []*idl.RenamePair{{
					Src: "/data/dbfast2",
					Dst: "/data/dbfast2_old",
				}},
			},
		).Return(&idl.RenameDirectoriesReply{}, nil)

		agentConns := []*hub.Connection{
			{nil, client1, "sdw1", nil},
			{nil, client2, "sdw2", nil},
		}

		err := hub.RenameSegmentDataDirs(agentConns, c, hub.OldSuffix, false)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("returns error on failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client := mock_idl.NewMockAgentClient(ctrl)
		client.EXPECT().RenameDirectories(
			gomock.Any(),
			&idl.RenameDirectoriesRequest{
				Pairs: []*idl.RenamePair{{
					Src: "/data/dbfast1",
					Dst: "/data/dbfast1_upgrade",
				}},
			},
		).Return(&idl.RenameDirectoriesReply{}, nil)

		expected := errors.New("permission denied")
		failedClient := mock_idl.NewMockAgentClient(ctrl)
		failedClient.EXPECT().RenameDirectories(
			gomock.Any(),
			&idl.RenameDirectoriesRequest{
				Pairs: []*idl.RenamePair{{
					Src: "/data/dbfast2",
					Dst: "/data/dbfast2_upgrade",
				}},
			},
		).Return(nil, expected)

		agentConns := []*hub.Connection{
			{nil, client, "sdw1", nil},
			{nil, failedClient, "sdw2", nil},
		}

		err := hub.RenameSegmentDataDirs(agentConns, c, hub.UpgradeSuffix, false)

		var multiErr *multierror.Error
		if !xerrors.As(err, &multiErr) {
			t.Fatalf("got error %#v, want type %T", err, multiErr)
		}

		if len(multiErr.Errors) != 1 {
			t.Errorf("received %d errors, want %d", len(multiErr.Errors), 1)
		}

		for _, err := range multiErr.Errors {
			if !xerrors.Is(err, expected) {
				t.Errorf("got error %#v, want %#v", expected, err)
			}
		}
	})

	t.Run("returns error when failing to get segments for host", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		c := hub.MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: utils.PrimaryRole},
		})

		agentConns := []*hub.Connection{
			{nil, mock_idl.NewMockAgentClient(ctrl), "localhost", nil},
		}

		err := hub.RenameSegmentDataDirs(agentConns, c, hub.UpgradeSuffix, false)

		var multiErr *multierror.Error
		if !xerrors.As(err, &multiErr) {
			t.Fatalf("got error %#v, want type %T", err, multiErr)
		}

		if len(multiErr.Errors) != 1 {
			t.Errorf("received %d errors, want %d", len(multiErr.Errors), 1)
		}

		expected := utils.UnknownHostError{Hostname: "localhost"}
		for _, err := range multiErr.Errors {
			if !xerrors.Is(err, expected) {
				t.Errorf("got error %#v, want %#v", err, expected)
			}
		}
	})
}
