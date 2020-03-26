package hub_test

import (
	"errors"
	"testing"

	"github.com/hashicorp/go-multierror"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestRenameDataDirs(t *testing.T) {
	t.Run("renames both source and target", func(t *testing.T) {
		numCalls := 0
		utils.System.Rename = func(src, dst string) error {
			if numCalls == 0 {

				expectedSrc := "/data/qddir/demoDataDir-1"
				if src != expectedSrc {
					t.Errorf("got %q want %q", src, expectedSrc)
				}

				expectedDst := "/data/qddir/demoDataDir-1_old"
				if dst != expectedDst {
					t.Errorf("got %q want %q", dst, expectedDst)
				}
				numCalls++

			} else if numCalls == 1 {

				expectedSrc := "/data/qddir/demoDataDir-1_ABC123-1"
				if src != expectedSrc {
					t.Errorf("got %q want %q", src, expectedSrc)
				}

				expectedDst := "/data/qddir/demoDataDir-1"
				if dst != expectedDst {
					t.Errorf("got %q want %q", dst, expectedDst)
				}
				numCalls++

			} else {
				t.Errorf("called too many times: %d", numCalls)
			}

			return nil
		}

		err := hub.RenameDataDirs("/data/qddir/demoDataDir-1", "/data/qddir/demoDataDir-1_ABC123-1")
		if err != nil {
			t.Errorf("unexpected error got %#v", err)
		}
	})

	t.Run("returns error when rename fails", func(t *testing.T) {
		expected := errors.New("permission denied")
		utils.System.Rename = func(src, dst string) error {
			return expected
		}

		err := hub.RenameDataDirs("/data/qddir/demoDataDir-1", "/data/qddir/demoDataDir-1_ABC123-1")
		if !xerrors.Is(err, expected) {
			t.Errorf("got %#v want %#v", err, expected)
		}
	})
}

func TestRenameSegmentDataDirs(t *testing.T) {
	testhelper.SetupTestLogger() // initialize gplog

	m := hub.RenameMap{
		"sdw1": {
			{
				Src: "/data/dbfast1/seg1_123ABC",
				Dst: "/data/dbfast1/seg1",
			},
			{
				Src: "/data/dbfast1/seg3_123ABC",
				Dst: "/data/dbfast1/seg3",
			},
		},
		"sdw2": {
			{
				Src: "/data/dbfast2/seg2_123ABC",
				Dst: "/data/dbfast2/seg2",
			},
			{
				Src: "/data/dbfast2/seg4_123ABC",
				Dst: "/data/dbfast2/seg4",
			},
		},
	}

	t.Run("issues agent commmand containing the specified pairs, skipping hosts with no pairs", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client1 := mock_idl.NewMockAgentClient(ctrl)
		client1.EXPECT().RenameDirectories(
			gomock.Any(),
			&idl.RenameDirectoriesRequest{
				Pairs: []*idl.RenamePair{{
					Src: "/data/dbfast1/seg1_123ABC",
					Dst: "/data/dbfast1/seg1",
				}, {
					Src: "/data/dbfast1/seg3_123ABC",
					Dst: "/data/dbfast1/seg3",
				}},
			},
		).Return(&idl.RenameDirectoriesReply{}, nil)

		client2 := mock_idl.NewMockAgentClient(ctrl)
		client2.EXPECT().RenameDirectories(
			gomock.Any(),
			&idl.RenameDirectoriesRequest{
				Pairs: []*idl.RenamePair{{
					Src: "/data/dbfast2/seg2_123ABC",
					Dst: "/data/dbfast2/seg2",
				}, {
					Src: "/data/dbfast2/seg4_123ABC",
					Dst: "/data/dbfast2/seg4",
				}},
			},
		).Return(&idl.RenameDirectoriesReply{}, nil)

		client3 := mock_idl.NewMockAgentClient(ctrl)
		// NOTE: we expect no call to the standby

		agentConns := []*hub.Connection{
			{nil, client1, "sdw1", nil},
			{nil, client2, "sdw2", nil},
			{nil, client3, "standby", nil},
		}

		err := hub.RenameSegmentDataDirs(agentConns, m)
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
			gomock.Any(),
		).Return(&idl.RenameDirectoriesReply{}, nil)

		expected := errors.New("permission denied")
		failedClient := mock_idl.NewMockAgentClient(ctrl)
		failedClient.EXPECT().RenameDirectories(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		agentConns := []*hub.Connection{
			{nil, client, "sdw1", nil},
			{nil, failedClient, "sdw2", nil},
		}

		err := hub.RenameSegmentDataDirs(agentConns, m)

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
}

func TestUpdateDataDirectories(t *testing.T) {
	// Prerequisites:
	// - a valid Source cluster
	// - a valid TargetInitializeConfig (XXX should be Target once we fix it)
	// - agentConns pointing to each host (set up per test)

	conf := new(hub.Config)

	conf.Source = hub.MustCreateCluster(t, []greenplum.SegConfig{
		{ContentID: -1, Hostname: "sdw1", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
		{ContentID: -1, Hostname: "standby", DataDir: "/data/standby", Role: greenplum.MirrorRole},

		{ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
		{ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		{ContentID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg3", Role: greenplum.PrimaryRole},
		{ContentID: 3, Hostname: "sdw2", DataDir: "/data/dbfast2/seg4", Role: greenplum.PrimaryRole},

		{ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast_mirror1/seg1", Role: greenplum.MirrorRole},
		{ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast_mirror2/seg2", Role: greenplum.MirrorRole},
		{ContentID: 2, Hostname: "sdw1", DataDir: "/data/dbfast_mirror1/seg3", Role: greenplum.MirrorRole},
		{ContentID: 3, Hostname: "sdw2", DataDir: "/data/dbfast_mirror2/seg4", Role: greenplum.MirrorRole},
	})

	conf.TargetInitializeConfig = hub.InitializeConfig{
		Master: greenplum.SegConfig{
			ContentID: -1, Hostname: "sdw1", DataDir: "/data/qddir/seg-1_123ABC-1", Role: greenplum.PrimaryRole,
		},
		Standby: greenplum.SegConfig{
			ContentID: -1, Hostname: "standby", DataDir: "/data/standby_123ABC", Role: greenplum.MirrorRole,
		},
		Primaries: []greenplum.SegConfig{
			{ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1_123ABC", Role: greenplum.PrimaryRole},
			{ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2_123ABC", Role: greenplum.PrimaryRole},
			{ContentID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg3_123ABC", Role: greenplum.PrimaryRole},
			{ContentID: 3, Hostname: "sdw2", DataDir: "/data/dbfast2/seg4_123ABC", Role: greenplum.PrimaryRole},
		},
		Mirrors: []greenplum.SegConfig{
			{ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast_mirror1/seg1_123ABC", Role: greenplum.MirrorRole},
			{ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast_mirror2/seg2_123ABC", Role: greenplum.MirrorRole},
			{ContentID: 2, Hostname: "sdw1", DataDir: "/data/dbfast_mirror1/seg3_123ABC", Role: greenplum.MirrorRole},
			{ContentID: 3, Hostname: "sdw2", DataDir: "/data/dbfast_mirror2/seg4_123ABC", Role: greenplum.MirrorRole},
		},
	}

	utils.System.Rename = func(src, dst string) error {
		return nil
	}

	t.Run("transmits segment rename requests to the correct agents in copy mode", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conf.UseLinkMode = false

		// We want the source's primaries and mirrors to be archived, but only
		// the target's upgraded primaries should be moved back to the source
		// locations.
		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		expectRenames(sdw1, []*idl.RenamePair{{
			Src: "/data/dbfast1/seg1",
			Dst: "/data/dbfast1/seg1_old",
		}, {
			Src: "/data/dbfast_mirror1/seg1",
			Dst: "/data/dbfast_mirror1/seg1_old",
		}, {
			Src: "/data/dbfast1/seg3",
			Dst: "/data/dbfast1/seg3_old",
		}, {
			Src: "/data/dbfast_mirror1/seg3",
			Dst: "/data/dbfast_mirror1/seg3_old",
		}})
		expectRenames(sdw1, []*idl.RenamePair{{
			Src: "/data/dbfast1/seg1_123ABC",
			Dst: "/data/dbfast1/seg1",
		}, {
			Src: "/data/dbfast1/seg3_123ABC",
			Dst: "/data/dbfast1/seg3",
		}})

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		expectRenames(sdw2, []*idl.RenamePair{{
			Src: "/data/dbfast2/seg2",
			Dst: "/data/dbfast2/seg2_old",
		}, {
			Src: "/data/dbfast_mirror2/seg2",
			Dst: "/data/dbfast_mirror2/seg2_old",
		}, {
			Src: "/data/dbfast2/seg4",
			Dst: "/data/dbfast2/seg4_old",
		}, {
			Src: "/data/dbfast_mirror2/seg4",
			Dst: "/data/dbfast_mirror2/seg4_old",
		}})
		expectRenames(sdw2, []*idl.RenamePair{{
			Src: "/data/dbfast2/seg2_123ABC",
			Dst: "/data/dbfast2/seg2",
		}, {
			Src: "/data/dbfast2/seg4_123ABC",
			Dst: "/data/dbfast2/seg4",
		}})

		standby := mock_idl.NewMockAgentClient(ctrl)
		expectRenames(standby, []*idl.RenamePair{{
			Src: "/data/standby",
			Dst: "/data/standby_old",
		}})

		agentConns := []*hub.Connection{
			{nil, sdw1, "sdw1", nil},
			{nil, sdw2, "sdw2", nil},
			{nil, standby, "standby", nil},
		}

		err := hub.UpdateDataDirectories(conf, agentConns)
		if err != nil {
			t.Errorf("UpdateDataDirectories() returned error: %+v", err)
		}
	})

	t.Run("transmits segment rename requests to the correct agents in link mode", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conf.UseLinkMode = true

		// Similar to copy mode, but we want deletion requests on the mirrors
		// and standby as opposed to archive requests.
		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		expectDeletes(sdw1, []string{
			"/data/dbfast_mirror1/seg1",
			"/data/dbfast_mirror1/seg3",
		})
		expectRenames(sdw1, []*idl.RenamePair{{
			Src: "/data/dbfast1/seg1",
			Dst: "/data/dbfast1/seg1_old",
		}, {
			Src: "/data/dbfast1/seg3",
			Dst: "/data/dbfast1/seg3_old",
		}})
		expectRenames(sdw1, []*idl.RenamePair{{
			Src: "/data/dbfast1/seg1_123ABC",
			Dst: "/data/dbfast1/seg1",
		}, {
			Src: "/data/dbfast1/seg3_123ABC",
			Dst: "/data/dbfast1/seg3",
		}})

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		expectDeletes(sdw2, []string{
			"/data/dbfast_mirror2/seg2",
			"/data/dbfast_mirror2/seg4",
		})
		expectRenames(sdw2, []*idl.RenamePair{{
			Src: "/data/dbfast2/seg2",
			Dst: "/data/dbfast2/seg2_old",
		}, {
			Src: "/data/dbfast2/seg4",
			Dst: "/data/dbfast2/seg4_old",
		}})
		expectRenames(sdw2, []*idl.RenamePair{{
			Src: "/data/dbfast2/seg2_123ABC",
			Dst: "/data/dbfast2/seg2",
		}, {
			Src: "/data/dbfast2/seg4_123ABC",
			Dst: "/data/dbfast2/seg4",
		}})

		standby := mock_idl.NewMockAgentClient(ctrl)
		expectDeletes(standby, []string{
			"/data/standby",
		})

		agentConns := []*hub.Connection{
			{nil, sdw1, "sdw1", nil},
			{nil, sdw2, "sdw2", nil},
			{nil, standby, "standby", nil},
		}

		err := hub.UpdateDataDirectories(conf, agentConns)
		if err != nil {
			t.Errorf("UpdateDataDirectories() returned error: %+v", err)
		}
	})
}

// expectRenames is syntactic sugar for setting up an expectation on
// AgentClient.RenameDirectories().
func expectRenames(client *mock_idl.MockAgentClient, pairs []*idl.RenamePair) {
	client.EXPECT().RenameDirectories(
		gomock.Any(),
		&idl.RenameDirectoriesRequest{Pairs: pairs},
	).Return(&idl.RenameDirectoriesReply{}, nil)
}

// expectDeletes is syntactic sugar for setting up an expectation on
// AgentClient.DeleteDirectories().
func expectDeletes(client *mock_idl.MockAgentClient, datadirs []string) {
	client.EXPECT().DeleteDirectories(
		gomock.Any(),
		&idl.DeleteDirectoriesRequest{Datadirs: datadirs},
	).Return(&idl.DeleteDirectoriesReply{}, nil)
}
