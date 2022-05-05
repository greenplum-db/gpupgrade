// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

func TestRenameSegmentDataDirs(t *testing.T) {
	testlog.SetupLogger()

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

	t.Run("issues agent commmand containing the specified pairs, skipping hosts with no pairs", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client1 := mock_idl.NewMockAgentClient(ctrl)
		client1.EXPECT().RenameDirectories(
			gomock.Any(),
			&idl.RenameDirectoriesRequest{
				Dirs: []*idl.RenameDirectories{{
					Source: "/data/dbfast1/seg1_123ABC",
					Target: "/data/dbfast1/seg1",
				}, {
					Source: "/data/dbfast1/seg3_123ABC",
					Target: "/data/dbfast1/seg3",
				}},
			},
		).Return(&idl.RenameDirectoriesReply{}, nil)

		client2 := mock_idl.NewMockAgentClient(ctrl)
		client2.EXPECT().RenameDirectories(
			gomock.Any(),
			&idl.RenameDirectoriesRequest{
				Dirs: []*idl.RenameDirectories{{
					Source: "/data/dbfast2/seg2_123ABC",
					Target: "/data/dbfast2/seg2",
				}, {
					Source: "/data/dbfast2/seg4_123ABC",
					Target: "/data/dbfast2/seg4",
				}},
			},
		).Return(&idl.RenameDirectoriesReply{}, nil)

		client3 := mock_idl.NewMockAgentClient(ctrl)
		// NOTE: we expect no call to the standby

		agentConns := []*idl.Connection{
			{AgentClient: client1, Hostname: "sdw1"},
			{AgentClient: client2, Hostname: "sdw2"},
			{AgentClient: client3, Hostname: "standby"},
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

		agentConns := []*idl.Connection{
			{AgentClient: client, Hostname: "sdw1"},
			{AgentClient: failedClient, Hostname: "sdw2"},
		}

		err := hub.RenameSegmentDataDirs(agentConns, m)

		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})
}

func TestUpdateDataDirectories(t *testing.T) {
	// Prerequisites:
	// - a valid Source cluster
	// - a valid Intermediate cluster
	// - agentConns pointing to each host (set up per test)

	conf := new(hub.Config)

	conf.Source = hub.MustCreateCluster(t, greenplum.SegConfigs{
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

	conf.Intermediate = hub.MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, Hostname: "sdw1", DataDir: "/data/qddir/seg-1_123ABC-1", Role: greenplum.PrimaryRole},
		{ContentID: -1, Hostname: "standby", DataDir: "/data/standby_123ABC", Role: greenplum.MirrorRole},

		{ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1_123ABC", Role: greenplum.PrimaryRole},
		{ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2_123ABC", Role: greenplum.PrimaryRole},
		{ContentID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg3_123ABC", Role: greenplum.PrimaryRole},
		{ContentID: 3, Hostname: "sdw2", DataDir: "/data/dbfast2/seg4_123ABC", Role: greenplum.PrimaryRole},

		{ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast_mirror1/seg1_123ABC", Role: greenplum.MirrorRole},
		{ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast_mirror2/seg2_123ABC", Role: greenplum.MirrorRole},
		{ContentID: 2, Hostname: "sdw1", DataDir: "/data/dbfast_mirror1/seg3_123ABC", Role: greenplum.MirrorRole},
		{ContentID: 3, Hostname: "sdw2", DataDir: "/data/dbfast_mirror2/seg4_123ABC", Role: greenplum.MirrorRole},
	})

	hub.RenameDirectories = func(source, target string) error {
		return nil
	}

	t.Run("renames master data directories", func(t *testing.T) {
		conf := new(hub.Config)

		sourceDataDir, targetDataDir, cleanup := testutils.MustCreateDataDirs(t)
		defer cleanup(t)

		conf.Source = hub.MustCreateCluster(t, greenplum.SegConfigs{
			{ContentID: -1, Hostname: "sdw1", DataDir: sourceDataDir, Role: greenplum.PrimaryRole},
		})

		conf.Intermediate = hub.MustCreateCluster(t, greenplum.SegConfigs{
			{ContentID: -1, Hostname: "sdw1", DataDir: targetDataDir, Role: greenplum.PrimaryRole},
		})

		hub.RenameDirectories = upgrade.RenameDirectories
		defer func() {
			hub.RenameDirectories = func(source, target string) error {
				return nil
			}
		}()

		err := hub.RenameDataDirectories(nil, conf.Source, conf.Intermediate)
		if err != nil {
			t.Errorf("UpdateDataDirectories() returned error: %+v", err)
		}

		testutils.VerifyRename(t, sourceDataDir, targetDataDir)
	})

	t.Run("returns error when renaming master data directories fails", func(t *testing.T) {
		expected := errors.New("permission denied")
		hub.RenameDirectories = func(source, target string) error {
			return expected
		}
		defer func() {
			hub.RenameDirectories = func(source, target string) error {
				return nil
			}
		}()

		err := hub.RenameDataDirectories(nil, conf.Source, conf.Intermediate)
		if !errors.Is(err, expected) {
			t.Errorf("got %#v want %#v", err, expected)
		}
	})

	t.Run("transmits segment rename requests to the correct agents in copy mode", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conf.LinkMode = false

		// We want the source's primaries and mirrors to be archived, but only
		// the target's upgraded primaries should be moved back to the source
		// locations.
		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		expectRenames(sdw1, []*idl.RenameDirectories{{
			Source: "/data/dbfast1/seg1",
			Target: "/data/dbfast1/seg1_123ABC",
		}, {
			Source: "/data/dbfast1/seg3",
			Target: "/data/dbfast1/seg3_123ABC",
		}, {
			Source: "/data/dbfast_mirror1/seg1",
			Target: "/data/dbfast_mirror1/seg1_123ABC",
		}, {
			Source: "/data/dbfast_mirror1/seg3",
			Target: "/data/dbfast_mirror1/seg3_123ABC",
		}})

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		expectRenames(sdw2, []*idl.RenameDirectories{{
			Source: "/data/dbfast2/seg2",
			Target: "/data/dbfast2/seg2_123ABC",
		}, {
			Source: "/data/dbfast2/seg4",
			Target: "/data/dbfast2/seg4_123ABC",
		}, {
			Source: "/data/dbfast_mirror2/seg2",
			Target: "/data/dbfast_mirror2/seg2_123ABC",
		}, {
			Source: "/data/dbfast_mirror2/seg4",
			Target: "/data/dbfast_mirror2/seg4_123ABC",
		}})

		standby := mock_idl.NewMockAgentClient(ctrl)
		expectRenames(standby, []*idl.RenameDirectories{{
			Source: "/data/standby",
			Target: "/data/standby_123ABC",
		}})

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
			{AgentClient: standby, Hostname: "standby"},
		}

		err := hub.RenameDataDirectories(agentConns, conf.Source, conf.Intermediate)
		if err != nil {
			t.Errorf("RenameDataDirectories() returned error: %+v", err)
		}
	})

	t.Run("transmits segment rename requests to the correct agents in link mode", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conf.LinkMode = true

		// Similar to copy mode, but we want deletion requests on the mirrors
		// and standby as opposed to archive requests.
		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		expectRenames(sdw1, []*idl.RenameDirectories{{
			Source: "/data/dbfast1/seg1",
			Target: "/data/dbfast1/seg1_123ABC",
		}, {
			Source: "/data/dbfast1/seg3",
			Target: "/data/dbfast1/seg3_123ABC",
		}, {
			Source: "/data/dbfast_mirror1/seg1",
			Target: "/data/dbfast_mirror1/seg1_123ABC",
		}, {
			Source: "/data/dbfast_mirror1/seg3",
			Target: "/data/dbfast_mirror1/seg3_123ABC",
		}})

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		expectRenames(sdw2, []*idl.RenameDirectories{{
			Source: "/data/dbfast2/seg2",
			Target: "/data/dbfast2/seg2_123ABC",
		}, {
			Source: "/data/dbfast2/seg4",
			Target: "/data/dbfast2/seg4_123ABC",
		}, {
			Source: "/data/dbfast_mirror2/seg2",
			Target: "/data/dbfast_mirror2/seg2_123ABC",
		}, {
			Source: "/data/dbfast_mirror2/seg4",
			Target: "/data/dbfast_mirror2/seg4_123ABC",
		}})

		standby := mock_idl.NewMockAgentClient(ctrl)
		expectRenames(standby, []*idl.RenameDirectories{{
			Source: "/data/standby",
			Target: "/data/standby_123ABC",
		}})

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
			{AgentClient: standby, Hostname: "standby"},
		}

		err := hub.RenameDataDirectories(agentConns, conf.Source, conf.Intermediate)
		if err != nil {
			t.Errorf("RenameDataDirectories() returned error: %+v", err)
		}
	})
}

// expectRenames is syntactic sugar for setting up an expectation on
// AgentClient.RenameDirectories().
func expectRenames(client *mock_idl.MockAgentClient, pairs []*idl.RenameDirectories) {
	client.EXPECT().RenameDirectories(
		gomock.Any(),
		equivalentRenameDirsRequest(&idl.RenameDirectoriesRequest{Dirs: pairs}),
	).Return(&idl.RenameDirectoriesReply{}, nil)
}

// equivalentRequest is a Matcher that can handle differences in order between
// two instances of DeleteTablespaceRequest.Dirs
func equivalentRenameDirsRequest(req *idl.RenameDirectoriesRequest) gomock.Matcher {
	return renameDirsReqMatcher{req}
}

type renameDirsReqMatcher struct {
	expected *idl.RenameDirectoriesRequest
}

func (r renameDirsReqMatcher) Matches(x interface{}) bool {
	actual, ok := x.(*idl.RenameDirectoriesRequest)
	if !ok {
		return false
	}

	// The key here is that Datadirs can be in any order. Sort them before
	// comparison.
	sort.Sort(renameDirectories(r.expected.GetDirs()))
	sort.Sort(renameDirectories(actual.GetDirs()))

	return reflect.DeepEqual(r.expected, actual)
}

func (r renameDirsReqMatcher) String() string {
	return fmt.Sprintf("is equivalent to %v", r.expected)
}

type renameDirectories []*idl.RenameDirectories

func (r renameDirectories) Len() int {
	return len(r)
}

func (r renameDirectories) Less(i, j int) bool {
	return r[i].Source > r[j].Source
}

func (r renameDirectories) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}
