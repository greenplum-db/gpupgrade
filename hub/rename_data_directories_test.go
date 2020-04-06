package hub_test

import (
	"errors"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

// Since this simple function is heavily relied upon, test it thoroughly.
func TestAlreadyRenamed(t *testing.T) {

	testhelper.SetupTestLogger() // initialize gplog

	cases := []struct {
		name             string
		clusterHasMirror bool
		deleteSource     bool
		deleteTarget     bool
		deleteArchiveDir bool
		expected         bool
	}{
		{name: "returns true when called properly setup with mirror",
			clusterHasMirror: true,
			deleteSource:     false,
			deleteTarget:     true,
			deleteArchiveDir: false,
			expected:         true,
		},
		{name: "returns false when called with mirror with removed source directory",
			clusterHasMirror: true,
			deleteSource:     true,
			deleteTarget:     true,
			deleteArchiveDir: false,
			expected:         false,
		},
		{name: "returns false when called with mirror with no archive directory",
			clusterHasMirror: true,
			deleteSource:     false,
			deleteTarget:     true,
			deleteArchiveDir: true,
			expected:         false,
		},
		{name: "returns false when called with mirror with present target directory",
			clusterHasMirror: true,
			deleteSource:     false,
			deleteTarget:     false,
			deleteArchiveDir: false,
			expected:         false,
		},
		{name: "returns true when called properly setup without mirror",
			clusterHasMirror: false,
			deleteSource:     true,
			deleteTarget:     true, // no mirror
			deleteArchiveDir: false,
			expected:         true,
		},
		{name: "returns false without mirror when source still in place",
			clusterHasMirror: false,
			deleteSource:     false,
			deleteTarget:     true, // no mirror
			deleteArchiveDir: false,
			expected:         false,
		},
		{name: "returns false without mirror when archive not there",
			clusterHasMirror: false,
			deleteSource:     true,
			deleteTarget:     true, // no mirror
			deleteArchiveDir: true,
			expected:         false,
		},
	}
	for _, c := range cases {

		source, target, tmpDir := testutils.SetupDataDirs(t)
		defer func() {
			os.RemoveAll(tmpDir)
		}()

		t.Run(c.name, func(t *testing.T) {
			if c.deleteSource {
				err := os.RemoveAll(source)
				if err != nil {
					t.Errorf("unexpected error: %#v", err)
				}
			}
			if c.deleteTarget {
				err := os.RemoveAll(target)
				if err != nil {
					t.Errorf("unexpected error: %#v", err)
				}
			}

			archiveDir := upgrade.ArchiveDirectoryForSource(source, testutils.UpgradeID)
			if !c.deleteArchiveDir {
				err := os.Mkdir(archiveDir, 0700)
				if err != nil {
					t.Errorf("unexpected error: %#v", err)
				}
			}

			if hub.AlreadyRenamed(source, target, archiveDir, !c.clusterHasMirror) != c.expected {
				t.Errorf("expected %v", c.expected)
			}

		})
	}

}

func TestRenameDataDirs(t *testing.T) {

	testhelper.SetupTestLogger() // initialize gplog

	cases := []struct {
		name       string
		iterations int
		moveBoth   bool
	}{
		{name: "renames source and target correctly",
			iterations: 1,
			moveBoth:   true,
		},
		{name: "renames source and target correctly and idempotence works",
			iterations: 2,
			moveBoth:   true,
		},
		{name: "renames source only correctly",
			iterations: 1,
		},
		{name: "renames source only and idempotence works",
			iterations: 2,
		},
	}
	for _, c := range cases {

		source, initialTarget, tmpDir := testutils.SetupDataDirs(t)
		defer func() {
			os.RemoveAll(tmpDir)
		}()

		t.Run(c.name, func(t *testing.T) {
			for i := 0; i < c.iterations; i++ {
				target := ""
				if c.moveBoth {
					target = initialTarget
				}

				err := hub.RenameDataDirs(source, target, testutils.UpgradeID)
				if err != nil {
					t.Errorf("iteration: %d: %v", i, err)
				}

				if c.moveBoth {
					if !hub.AlreadyRenamed(source, target, upgrade.ArchiveDirectoryForSource(source, testutils.UpgradeID), false) {
						t.Errorf("expected true")
					}
				} else {
					if !hub.AlreadyRenamed(source, target, upgrade.ArchiveDirectoryForSource(source, testutils.UpgradeID), true) {
						t.Errorf("expected true")
					}
				}
			}
		})
	}

	t.Run("returns error when rename fails", func(t *testing.T) {
		expected := errors.New("permission denied")
		utils.System.Rename = func(src, dst string) error {
			return expected
		}

		err := hub.RenameDataDirs("/data/qddir/demoDataDir-1", "/data/qddir/demoDataDir-1_CgAAAAAAAAA-1", testutils.UpgradeID)
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
				Source: "/data/dbfast1/seg1_123ABC",
				Target: "/data/dbfast1/seg1",
			},
			{
				Source: "/data/dbfast1/seg3_123ABC",
				Target: "/data/dbfast1/seg3",
			},
		},
		"sdw2": {
			{
				Source: "/data/dbfast2/seg2_123ABC",
				Target: "/data/dbfast2/seg2",
			},
			{
				Source: "/data/dbfast2/seg4_123ABC",
				Target: "/data/dbfast2/seg4",
			},
		},
	}

	t.Run("issues agent command containing the specified dataDirs, skipping hosts with no dataDirs", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client1 := mock_idl.NewMockAgentClient(ctrl)
		client1.EXPECT().RenameDataDirectories(
			gomock.Any(),
			&idl.RenameDataDirectoriesRequest{
				DataDirs: []*idl.RenameDataDirs{{
					Source: "/data/dbfast1/seg1_123ABC",
					Target: "/data/dbfast1/seg1",
				}, {
					Source: "/data/dbfast1/seg3_123ABC",
					Target: "/data/dbfast1/seg3",
				}},
			},
		).Return(&idl.RenameDataDirectoriesReply{}, nil)

		client2 := mock_idl.NewMockAgentClient(ctrl)
		client2.EXPECT().RenameDataDirectories(
			gomock.Any(),
			&idl.RenameDataDirectoriesRequest{
				DataDirs: []*idl.RenameDataDirs{{
					Source: "/data/dbfast2/seg2_123ABC",
					Target: "/data/dbfast2/seg2",
				}, {
					Source: "/data/dbfast2/seg4_123ABC",
					Target: "/data/dbfast2/seg4",
				}},
			},
		).Return(&idl.RenameDataDirectoriesReply{}, nil)

		client3 := mock_idl.NewMockAgentClient(ctrl)
		// NOTE: we expect no call to the standby

		agentConns := []*hub.Connection{
			{nil, client1, "sdw1", nil},
			{nil, client2, "sdw2", nil},
			{nil, client3, "standby", nil},
		}

		err := hub.RenameSegmentDataDirs(agentConns, m, 0)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("returns error on an agent failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client := mock_idl.NewMockAgentClient(ctrl)
		client.EXPECT().RenameDataDirectories(
			gomock.Any(),
			gomock.Any(),
		).Return(&idl.RenameDataDirectoriesReply{}, nil)

		expected := errors.New("permission denied")
		failedClient := mock_idl.NewMockAgentClient(ctrl)
		failedClient.EXPECT().RenameDataDirectories(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		agentConns := []*hub.Connection{
			{nil, client, "sdw1", nil},
			{nil, failedClient, "sdw2", nil},
		}

		err := hub.RenameSegmentDataDirs(agentConns, m, 0)

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
	defer func() {
		utils.System.Rename = os.Rename
	}()

	t.Run("transmits segment rename requests to the correct agents in copy mode", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conf.UseLinkMode = false

		// We want the source's primaries and mirrors to be archived, but only
		// the target's upgraded primaries should be moved back to the source
		// locations.
		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		expectRenames(sdw1, []*idl.RenameDataDirs{{
			Source: "/data/dbfast1/seg1",
			Target: "/data/dbfast1/seg1_123ABC",
		}, {
			Source: "/data/dbfast_mirror1/seg1",
		}, {
			Source: "/data/dbfast1/seg3",
			Target: "/data/dbfast1/seg3_123ABC",
		}, {
			Source: "/data/dbfast_mirror1/seg3",
		}})

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		expectRenames(sdw2, []*idl.RenameDataDirs{{
			Source: "/data/dbfast2/seg2",
			Target: "/data/dbfast2/seg2_123ABC",
		}, {
			Source: "/data/dbfast_mirror2/seg2",
		}, {
			Source: "/data/dbfast2/seg4",
			Target: "/data/dbfast2/seg4_123ABC",
		}, {
			Source: "/data/dbfast_mirror2/seg4",
		}})

		standby := mock_idl.NewMockAgentClient(ctrl)
		expectRenames(standby, []*idl.RenameDataDirs{{
			Source: "/data/standby",
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
		expectRenames(sdw1, []*idl.RenameDataDirs{{
			Source: "/data/dbfast1/seg1",
			Target: "/data/dbfast1/seg1_123ABC",
		}, {
			Source: "/data/dbfast1/seg3",
			Target: "/data/dbfast1/seg3_123ABC",
		}})

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		expectDeletes(sdw2, []string{
			"/data/dbfast_mirror2/seg2",
			"/data/dbfast_mirror2/seg4",
		})
		expectRenames(sdw2, []*idl.RenameDataDirs{{
			Source: "/data/dbfast2/seg2",
			Target: "/data/dbfast2/seg2_123ABC",
		}, {
			Source: "/data/dbfast2/seg4",
			Target: "/data/dbfast2/seg4_123ABC",
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
// AgentClient.RenameDataDirectories().
func expectRenames(client *mock_idl.MockAgentClient, dataDirs []*idl.RenameDataDirs) {
	client.EXPECT().RenameDataDirectories(
		gomock.Any(),
		&idl.RenameDataDirectoriesRequest{DataDirs: dataDirs},
	).Return(&idl.RenameDataDirectoriesReply{}, nil)
}

// expectDeletes is syntactic sugar for setting up an expectation on
// AgentClient.DeleteDirectories().
func expectDeletes(client *mock_idl.MockAgentClient, datadirs []string) {
	client.EXPECT().DeleteDirectories(
		gomock.Any(),
		&idl.DeleteDirectoriesRequest{Datadirs: datadirs},
	).Return(&idl.DeleteDirectoriesReply{}, nil)
}
