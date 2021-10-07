//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func TestRsyncMirrorDataDirsOnSegments(t *testing.T) {
	source := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "master", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby", Port: 16432, Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Port: 25433, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg1", Port: 25434, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Port: 25435, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg2", Port: 25436, Role: greenplum.MirrorRole},
	})

	intermediate := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "master", DataDir: "/data/qddir/seg.HqtFHX54y0o.-1", Port: 50432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby.HqtFHX54y0o", Port: 50433, Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg.HqtFHX54y0o.1", Port: 50434, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg.HqtFHX54y0o.1", Port: 50435, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg.HqtFHX54y0o.2", Port: 50436, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg.HqtFHX54y0o.2", Port: 50437, Role: greenplum.MirrorRole},
	})

	t.Run("succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().RsyncDataDirectories(
			gomock.Any(),
			&idl.RsyncRequest{
				Options: []*idl.RsyncRequest_RsyncOptions{
					{
						Sources:         []string{"/data/dbfast1/seg.HqtFHX54y0o.1", "/data/dbfast1/seg1"},
						Destination:     "/data/dbfast_mirror1",
						DestinationHost: "sdw2",
						Options:         []string{"--archive", "--delete", "--hard-links", "--size-only", "--no-inc-recursive"},
					}},
			},
		).Return(&idl.RsyncReply{}, nil)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().RsyncDataDirectories(
			gomock.Any(),
			&idl.RsyncRequest{
				Options: []*idl.RsyncRequest_RsyncOptions{
					{
						Sources:         []string{"/data/dbfast2/seg.HqtFHX54y0o.2", "/data/dbfast2/seg2"},
						Destination:     "/data/dbfast_mirror2",
						DestinationHost: "sdw1",
						Options:         []string{"--archive", "--delete", "--hard-links", "--size-only", "--no-inc-recursive"},
					}},
			},
		).Return(&idl.RsyncReply{}, nil)

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err := hub.RsyncMirrorDataDirsOnSegments(agentConns, intermediate, source)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("returns errors when failing on segments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		expected := errors.New("permission denied")
		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().RsyncDataDirectories(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().RsyncDataDirectories(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err := hub.RsyncMirrorDataDirsOnSegments(agentConns, intermediate, source)
		var errs errorlist.Errors
		if !xerrors.As(err, &errs) {
			t.Fatalf("error %#v does not contain type %T", err, errs)
		}

		if len(errs) != 2 {
			t.Fatalf("got error count %d, want %d", len(errs), 2)
		}

		for _, err := range errs {
			if !errors.Is(err, expected) {
				t.Errorf("got error %#v, want %#v", err, expected)
			}
		}
	})
}

func TestRsyncAndRenameMirrorTablespacesOnSegments(t *testing.T) {
	source := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "master", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby", Port: 16432, Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Port: 25433, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg1", Port: 25434, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Port: 25435, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg2", Port: 25436, Role: greenplum.MirrorRole},
	})
	source.Tablespaces = testutils.CreateTablespaces()

	intermediate := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "master", DataDir: "/data/qddir/seg.HqtFHX54y0o.-1", Port: 50432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby.HqtFHX54y0o", Port: 50433, Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg.HqtFHX54y0o.1", Port: 50434, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg.HqtFHX54y0o.1", Port: 50435, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg.HqtFHX54y0o.2", Port: 50436, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg.HqtFHX54y0o.2", Port: 50437, Role: greenplum.MirrorRole},
	})

	t.Run("succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().RsyncTablespaceDirectories(
			gomock.Any(),
			&idl.RsyncRequest{
				Options: []*idl.RsyncRequest_RsyncOptions{
					{
						Sources:         []string{"/tmp/user_ts/p1/16384/"},
						Destination:     "/tmp/user_ts/m1/16384",
						DestinationHost: "sdw2",
						Options:         []string{"--archive", "--delete", "--hard-links", "--size-only", "--no-inc-recursive"},
					}},
			},
		).Return(&idl.RsyncReply{}, nil)
		sdw1.EXPECT().RenameTablespaces(
			gomock.Any(),
			&idl.RenameTablespacesRequest{
				RenamePairs: []*idl.RenameTablespacesRequest_RenamePair{
					{
						Source:      "/tmp/user_ts/m1/16384/3",
						Destination: "/tmp/user_ts/p1/16384/4",
					}},
			},
		).Return(&idl.RenameTablespacesReply{}, nil)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().RsyncTablespaceDirectories(
			gomock.Any(),
			&idl.RsyncRequest{
				Options: []*idl.RsyncRequest_RsyncOptions{
					{
						Sources:         []string{"/tmp/user_ts/p2/16384/"},
						Destination:     "/tmp/user_ts/m2/16384",
						DestinationHost: "sdw1",
						Options:         []string{"--archive", "--delete", "--hard-links", "--size-only", "--no-inc-recursive"},
					}},
			},
		).Return(&idl.RsyncReply{}, nil)
		sdw2.EXPECT().RenameTablespaces(
			gomock.Any(),
			&idl.RenameTablespacesRequest{
				RenamePairs: []*idl.RenameTablespacesRequest_RenamePair{
					{
						Source:      "/tmp/user_ts/m2/16384/5",
						Destination: "/tmp/user_ts/p2/16384/6",
					}},
			},
		).Return(&idl.RenameTablespacesReply{}, nil)

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err := hub.RsyncAndRenameMirrorTablespacesOnSegments(agentConns, source, intermediate)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("returns error when failing to rsync tablespaces on segments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		expected := errors.New("permission denied")
		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().RsyncTablespaceDirectories(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().RsyncTablespaceDirectories(
			gomock.Any(),
			&idl.RsyncRequest{
				Options: []*idl.RsyncRequest_RsyncOptions{
					{
						Sources:         []string{"/tmp/user_ts/p2/16384/"},
						Destination:     "/tmp/user_ts/m2/16384",
						DestinationHost: "sdw1",
						Options:         []string{"--archive", "--delete", "--hard-links", "--size-only", "--no-inc-recursive"},
					}},
			},
		).Return(&idl.RsyncReply{}, nil)
		sdw2.EXPECT().RenameTablespaces(
			gomock.Any(),
			&idl.RenameTablespacesRequest{
				RenamePairs: []*idl.RenameTablespacesRequest_RenamePair{
					{
						Source:      "/tmp/user_ts/m2/16384/5",
						Destination: "/tmp/user_ts/p2/16384/6",
					}},
			},
		).Return(&idl.RenameTablespacesReply{}, nil)

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err := hub.RsyncAndRenameMirrorTablespacesOnSegments(agentConns, source, intermediate)
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})

	t.Run("returns error when failing to rename tablespaces on segments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		expected := errors.New("permission denied")
		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().RsyncTablespaceDirectories(
			gomock.Any(),
			&idl.RsyncRequest{
				Options: []*idl.RsyncRequest_RsyncOptions{
					{
						Sources:         []string{"/tmp/user_ts/p1/16384/"},
						Destination:     "/tmp/user_ts/m1/16384",
						DestinationHost: "sdw2",
						Options:         []string{"--archive", "--delete", "--hard-links", "--size-only", "--no-inc-recursive"},
					}},
			},
		).Return(&idl.RsyncReply{}, nil)
		sdw1.EXPECT().RenameTablespaces(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().RsyncTablespaceDirectories(
			gomock.Any(),
			&idl.RsyncRequest{
				Options: []*idl.RsyncRequest_RsyncOptions{
					{
						Sources:         []string{"/tmp/user_ts/p2/16384/"},
						Destination:     "/tmp/user_ts/m2/16384",
						DestinationHost: "sdw1",
						Options:         []string{"--archive", "--delete", "--hard-links", "--size-only", "--no-inc-recursive"},
					}},
			},
		).Return(&idl.RsyncReply{}, nil)
		sdw2.EXPECT().RenameTablespaces(
			gomock.Any(),
			&idl.RenameTablespacesRequest{
				RenamePairs: []*idl.RenameTablespacesRequest_RenamePair{
					{
						Source:      "/tmp/user_ts/m2/16384/5",
						Destination: "/tmp/user_ts/p2/16384/6",
					}},
			},
		).Return(&idl.RenameTablespacesReply{}, nil)

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err := hub.RsyncAndRenameMirrorTablespacesOnSegments(agentConns, source, intermediate)
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})
}
