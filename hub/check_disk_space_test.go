// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils/disk"
)

func TestCheckDiskSpace_OnMaster(t *testing.T) {
	source := hub.MustCreateCluster(t, []greenplum.SegConfig{
		{ContentID: -1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
	})

	tablespaces := greenplum.Tablespaces{}

	t.Run("returns no error or usage when checking disk usage on master succeeds", func(t *testing.T) {
		err := hub.CheckDiskSpace(step.DevNullStream, []*hub.Connection{}, 0, source, tablespaces)
		if err == nil {
			t.Errorf("unexpected error %#v", err)
		}
	})

	t.Run("errors when checking disk usage on master fails", func(t *testing.T) {
		expected := errors.New("permission denied")
		hub.CheckDiskUsageFunc = func(streams step.OutStreams, d disk.Disk, requiredRatio float64, paths ...string) (disk.FileSystemDiskUsage, error) {
			return nil, expected
		}

		err := hub.CheckDiskSpace(step.DevNullStream, []*hub.Connection{}, 0, source, tablespaces)
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})

	t.Run("returns usage when checking disk usage on master", func(t *testing.T) {
		usage := disk.FileSystemDiskUsage{
			&idl.CheckDiskSpaceReply_DiskUsage{
				Fs:        "/",
				Host:      "mdw",
				Available: 1024,
				Required:  2048,
			}}

		hub.CheckDiskUsageFunc = func(streams step.OutStreams, d disk.Disk, requiredRatio float64, paths ...string) (disk.FileSystemDiskUsage, error) {
			return usage, nil
		}

		err := hub.CheckDiskSpace(step.DevNullStream, []*hub.Connection{}, 0, source, tablespaces)
		expected := disk.NewSpaceUsageError(usage)
		if !reflect.DeepEqual(err, expected) {
			t.Errorf("returned %v want %v", err, expected)
		}
	})
}

func TestCheckDiskSpace_OnSegments(t *testing.T) {
	source := hub.MustCreateCluster(t, []greenplum.SegConfig{
		{DbID: 1, ContentID: -1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
		{DbID: 2, ContentID: -1, Hostname: "smdw", DataDir: "/data/standby", Role: "m"},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast/seg1", Role: "p"},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg1", Role: "m"},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast/seg2", Role: "p"},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg2", Role: "m"},
	})

	tablespaces := testutils.CreateTablespaces()

	hub.CheckDiskUsageFunc = func(streams step.OutStreams, d disk.Disk, requiredRatio float64, paths ...string) (disk.FileSystemDiskUsage, error) {
		return nil, nil
	}

	t.Run("returns no error or usage when checking disk usage on segment hosts succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		diskFreeRatio := 0.3

		smdw := mock_idl.NewMockAgentClient(ctrl)
		smdw.EXPECT().CheckDiskSpace(
			gomock.Any(),
			&idl.CheckSegmentDiskSpaceRequest{
				DiskFreeRatio: diskFreeRatio,
				Dirs:          []string{"/data/standby", "/tmp/user_ts/m/standby/16384"},
			},
		).Return(&idl.CheckDiskSpaceReply{}, nil)

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().CheckDiskSpace(
			gomock.Any(),
			&idl.CheckSegmentDiskSpaceRequest{
				DiskFreeRatio: diskFreeRatio,
				Dirs:          []string{"/data/dbfast/seg1", "/tmp/user_ts/p1/16384", "/data/dbfast_mirror2/seg2", "/tmp/user_ts/m2/16384"},
			},
		).Return(&idl.CheckDiskSpaceReply{}, nil)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().CheckDiskSpace(
			gomock.Any(),
			&idl.CheckSegmentDiskSpaceRequest{
				DiskFreeRatio: diskFreeRatio,
				Dirs:          []string{"/data/dbfast_mirror1/seg1", "/tmp/user_ts/m1/16384", "/data/dbfast/seg2", "/tmp/user_ts/p2/16384"},
			},
		).Return(&idl.CheckDiskSpaceReply{}, nil)

		agentConns := []*hub.Connection{
			{nil, smdw, "smdw", nil},
			{nil, sdw1, "sdw1", nil},
			{nil, sdw2, "sdw2", nil},
		}

		err := hub.CheckDiskSpace(step.DevNullStream, agentConns, diskFreeRatio, source, tablespaces)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}
	})

	t.Run("errors when checking disk usage on segment hosts fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		expected := errors.New("permission denied")
		failedClient := mock_idl.NewMockAgentClient(ctrl)
		failedClient.EXPECT().CheckDiskSpace(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		agentConns := []*hub.Connection{
			{nil, failedClient, "sdw1", nil},
		}

		err := hub.CheckDiskSpace(step.DevNullStream, agentConns, 0, source, tablespaces)
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})

	t.Run("returns usage when checking disk usage on segment hosts", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		usage := disk.FileSystemDiskUsage{
			&idl.CheckDiskSpaceReply_DiskUsage{
				Fs:        "/",
				Host:      "smdw",
				Available: 1024,
				Required:  2048,
			}}
		failedClient := mock_idl.NewMockAgentClient(ctrl)
		failedClient.EXPECT().CheckDiskSpace(
			gomock.Any(),
			gomock.Any(),
		).Return(&idl.CheckDiskSpaceReply{Usage: usage}, nil)

		agentConns := []*hub.Connection{
			{nil, failedClient, "smdw", nil},
		}

		err := hub.CheckDiskSpace(step.DevNullStream, agentConns, 0, source, tablespaces)
		expected := disk.NewSpaceUsageError(usage)
		if !reflect.DeepEqual(err, expected) {
			t.Errorf("returned %v want %v", err, expected)
		}
	})

	t.Run("does not check on segments if there are no segments to check", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().CheckDiskSpace(
			gomock.Any(),
			gomock.Any(),
		).Times(0) // expected to not be called for cluster with no segments

		agentConns := []*hub.Connection{
			{nil, sdw2, "sdw2", nil},
		}

		masterOnlyCluster := hub.MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
		})

		err := hub.CheckDiskSpace(step.DevNullStream, agentConns, 0, masterOnlyCluster, tablespaces)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}
	})
}
