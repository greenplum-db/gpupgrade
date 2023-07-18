// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
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
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils/disk"
)

func CoordinatorHostCheckDiskUsagePasses(streams step.OutStreams, d disk.Disk, requiredRatio float64, paths ...string) (disk.FileSystemDiskUsage, error) {
	return nil, nil
}

func CoordinatorHostErrorsWith(expected error) disk.CheckUsageType {
	return func(streams step.OutStreams, d disk.Disk, requiredRatio float64, paths ...string) (disk.FileSystemDiskUsage, error) {
		return nil, expected
	}
}

func CoordinatorHostReturnsUsage(expected disk.FileSystemDiskUsage) disk.CheckUsageType {
	return func(streams step.OutStreams, d disk.Disk, requiredRatio float64, paths ...string) (disk.FileSystemDiskUsage, error) {
		return expected, nil
	}
}

func TestCheckDiskSpace_OnCoordinator(t *testing.T) {
	source := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
	})

	tablespaces := greenplum.Tablespaces{}

	t.Run("does not return disk usage or any errors when checking disk usage on coordinator succeeds", func(t *testing.T) {
		hub.SetCheckDiskUsage(CoordinatorHostCheckDiskUsagePasses)
		defer hub.ResetCheckDiskUsage()

		err := hub.CheckDiskSpace(step.DevNullStream, []*idl.Connection{}, 0, source, tablespaces)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}
	})

	t.Run("errors when checking disk usage on coordinator fails", func(t *testing.T) {
		expected := errors.New("permission denied")
		hub.SetCheckDiskUsage(CoordinatorHostErrorsWith(expected))
		defer hub.ResetCheckDiskUsage()

		err := hub.CheckDiskSpace(step.DevNullStream, []*idl.Connection{}, 0, source, tablespaces)
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})

	t.Run("returns usage when checking disk usage on coordinator", func(t *testing.T) {
		usage := &idl.CheckDiskSpaceReply_DiskUsage{
			Fs:        "/",
			Host:      "mdw",
			Available: 1024,
			Required:  2048,
		}
		hub.SetCheckDiskUsage(CoordinatorHostReturnsUsage(disk.FileSystemDiskUsage{usage}))
		defer hub.ResetCheckDiskUsage()

		err := hub.CheckDiskSpace(step.DevNullStream, []*idl.Connection{}, 0, source, tablespaces)
		expected := disk.NewSpaceUsageErrorFromUsage(usage)
		if !reflect.DeepEqual(err, expected) {
			t.Errorf("returned %v want %v", err, expected)
		}
	})
}

func TestCheckDiskSpace_OnSegments(t *testing.T) {
	source := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "smdw", DataDir: "/data/standby", Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast/seg1", Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg1", Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast/seg2", Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg2", Role: greenplum.MirrorRole},
	})

	tablespaces := testutils.CreateTablespaces()

	hub.SetCheckDiskUsage(CoordinatorHostCheckDiskUsagePasses)
	defer hub.ResetCheckDiskUsage()

	t.Run("returns no error or usage when checking disk usage on segment hosts succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		diskFreeRatio := 0.3

		smdw := mock_idl.NewMockAgentClient(ctrl)
		smdw.EXPECT().CheckDiskSpace(
			gomock.Any(),
			equivalentCheckDiskRequest(&idl.CheckSegmentDiskSpaceRequest{
				DiskFreeRatio: diskFreeRatio,
				Dirs:          []string{"/data/standby", "/tmp/user_ts/m/standby/16384"},
			}),
		).Return(&idl.CheckDiskSpaceReply{}, nil)

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().CheckDiskSpace(
			gomock.Any(),
			equivalentCheckDiskRequest(&idl.CheckSegmentDiskSpaceRequest{
				DiskFreeRatio: diskFreeRatio,
				Dirs:          []string{"/data/dbfast/seg1", "/tmp/user_ts/p1/16384", "/data/dbfast_mirror2/seg2", "/tmp/user_ts/m2/16384"},
			}),
		).Return(&idl.CheckDiskSpaceReply{}, nil)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().CheckDiskSpace(
			gomock.Any(),
			equivalentCheckDiskRequest(&idl.CheckSegmentDiskSpaceRequest{
				DiskFreeRatio: diskFreeRatio,
				Dirs:          []string{"/data/dbfast_mirror1/seg1", "/tmp/user_ts/m1/16384", "/data/dbfast/seg2", "/tmp/user_ts/p2/16384"},
			}),
		).Return(&idl.CheckDiskSpaceReply{}, nil)

		agentConns := []*idl.Connection{
			{AgentClient: smdw, Hostname: "smdw"},
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
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

		agentConns := []*idl.Connection{
			{AgentClient: failedClient, Hostname: "sdw1"},
		}

		err := hub.CheckDiskSpace(step.DevNullStream, agentConns, 0, source, tablespaces)
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})

	t.Run("returns usage when checking disk usage on segment hosts", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		usage := &idl.CheckDiskSpaceReply_DiskUsage{
			Fs:        "/",
			Host:      "smdw",
			Available: 1024,
			Required:  2048,
		}

		failedClient := mock_idl.NewMockAgentClient(ctrl)
		failedClient.EXPECT().CheckDiskSpace(
			gomock.Any(),
			gomock.Any(),
		).Return(&idl.CheckDiskSpaceReply{Usages: disk.FileSystemDiskUsage{usage}}, nil)

		agentConns := []*idl.Connection{
			{AgentClient: failedClient, Hostname: "smdw"},
		}

		err := hub.CheckDiskSpace(step.DevNullStream, agentConns, 0, source, tablespaces)
		expected := disk.NewSpaceUsageErrorFromUsage(usage)
		if !reflect.DeepEqual(err, expected) {
			t.Errorf("returned %v want %v", err, expected)
		}
	})

	t.Run("combines usage across all hosts and removes duplicate usage between coordinator and segments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mdwUsage := disk.FileSystemDiskUsage{
			&idl.CheckDiskSpaceReply_DiskUsage{
				Fs:        "/data",
				Host:      "primary",
				Available: 1024,
				Required:  2048,
			}}
		hub.SetCheckDiskUsage(CoordinatorHostReturnsUsage(mdwUsage))
		defer hub.ResetCheckDiskUsage()

		primaryUsage := disk.FileSystemDiskUsage{
			&idl.CheckDiskSpaceReply_DiskUsage{
				Fs:        "/data",
				Host:      "primary",
				Available: 1024,
				Required:  2048,
			}}
		primary := mock_idl.NewMockAgentClient(ctrl)
		primary.EXPECT().CheckDiskSpace(
			gomock.Any(),
			gomock.Any(),
		).Return(&idl.CheckDiskSpaceReply{Usages: primaryUsage}, nil)

		mirrorUsage := disk.FileSystemDiskUsage{
			&idl.CheckDiskSpaceReply_DiskUsage{
				Fs:        "/data",
				Host:      "mirror",
				Available: 2024,
				Required:  4048,
			}}
		mirror := mock_idl.NewMockAgentClient(ctrl)
		mirror.EXPECT().CheckDiskSpace(
			gomock.Any(),
			gomock.Any(),
		).Return(&idl.CheckDiskSpaceReply{Usages: mirrorUsage}, nil)

		agentConns := []*idl.Connection{
			{AgentClient: primary, Hostname: "primary"},
			{AgentClient: mirror, Hostname: "mirror"},
		}

		sourceCluster := hub.MustCreateCluster(t, greenplum.SegConfigs{
			{DbID: 1, ContentID: -1, Hostname: "primary", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
			{DbID: 2, ContentID: -1, Hostname: "mirror", DataDir: "/data/standby", Role: greenplum.MirrorRole},
			{DbID: 3, ContentID: 0, Hostname: "primary", DataDir: "/data/dbfast/seg1", Role: greenplum.PrimaryRole},
			{DbID: 4, ContentID: 0, Hostname: "mirror", DataDir: "/data/dbfast_mirror1/seg1", Role: greenplum.MirrorRole},
			{DbID: 5, ContentID: 1, Hostname: "primary", DataDir: "/data/dbfast/seg2", Role: greenplum.PrimaryRole},
			{DbID: 6, ContentID: 1, Hostname: "mirror", DataDir: "/data/dbfast_mirror2/seg2", Role: greenplum.MirrorRole},
		})

		err := hub.CheckDiskSpace(step.DevNullStream, agentConns, 0, sourceCluster, tablespaces)
		expected := [][]string{
			{"Hostname", "Filesystem", "Shortfall", "Available", "Required"},
			{"mirror", "/data", disk.FormatBytes(2024), disk.FormatBytes(2024), disk.FormatBytes(4048)},
			{"primary", "/data", disk.FormatBytes(1024), disk.FormatBytes(1024), disk.FormatBytes(2048)},
		}

		var spaceUsageErr *disk.SpaceUsageErr
		if errors.As(err, &spaceUsageErr) {
			if !reflect.DeepEqual(spaceUsageErr.Table(), expected) {
				t.Errorf("returned %v want %v", spaceUsageErr.Table(), expected)
			}
		}
	})

	t.Run("does not check on segments if there are no segments to check", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		hub.SetCheckDiskUsage(CoordinatorHostCheckDiskUsagePasses)
		defer hub.ResetCheckDiskUsage()

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().CheckDiskSpace(
			gomock.Any(),
			gomock.Any(),
		).Times(0) // expected to not be called for cluster with no segments

		agentConns := []*idl.Connection{
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		coordinatorOnlyCluster := hub.MustCreateCluster(t, greenplum.SegConfigs{
			{ContentID: -1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
		})

		err := hub.CheckDiskSpace(step.DevNullStream, agentConns, 0, coordinatorOnlyCluster, tablespaces)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}
	})
}

// equivalentCheckDiskRequest is a Matcher that can handle differences in order between
// two instances of DeleteTablespaceRequest.Dirs
func equivalentCheckDiskRequest(req *idl.CheckSegmentDiskSpaceRequest) gomock.Matcher {
	return reqCheckDiskMatcher{req}
}

type reqCheckDiskMatcher struct {
	expected *idl.CheckSegmentDiskSpaceRequest
}

func (r reqCheckDiskMatcher) Matches(x interface{}) bool {
	actual, ok := x.(*idl.CheckSegmentDiskSpaceRequest)
	if !ok {
		return false
	}

	// The key here is that Datadirs can be in any order. Sort them before
	// comparison.
	sort.Strings(r.expected.Dirs)
	sort.Strings(actual.Dirs)

	return reflect.DeepEqual(r.expected, actual)
}

func (r reqCheckDiskMatcher) String() string {
	return fmt.Sprintf("is equivalent to %v", r.expected)
}
