// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/golang/mock/gomock"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
)

func TestDeleteSegmentDataDirs(t *testing.T) {
	primarySegConfigs := greenplum.SegConfigs{
		{ContentID: -1, DbID: 0, Port: 25431, Hostname: "coordinator", DataDir: "/data/qddir", Role: greenplum.PrimaryRole},
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		{ContentID: 2, DbID: 4, Port: 25434, Hostname: "sdw1", DataDir: "/data/dbfast1/seg3", Role: greenplum.PrimaryRole},
		{ContentID: 3, DbID: 5, Port: 25435, Hostname: "sdw2", DataDir: "/data/dbfast2/seg4", Role: greenplum.PrimaryRole},
	}

	testlog.SetupLogger()

	t.Run("DeleteCoordinatorAndPrimaryDataDirectories", func(t *testing.T) {
		t.Run("deletes coordinator and primary data directories", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			sdw1Client := mock_idl.NewMockAgentClient(ctrl)
			sdw1Client.EXPECT().DeleteDataDirectories(
				gomock.Any(),
				equivalentDeleteDataDirsRequest(&idl.DeleteDataDirectoriesRequest{Datadirs: []string{
					"/data/dbfast1/seg1",
					"/data/dbfast1/seg3",
				}},
				)).Return(&idl.DeleteDataDirectoriesReply{}, nil)

			sdw2Client := mock_idl.NewMockAgentClient(ctrl)
			sdw2Client.EXPECT().DeleteDataDirectories(
				gomock.Any(),
				equivalentDeleteDataDirsRequest(&idl.DeleteDataDirectoriesRequest{Datadirs: []string{
					"/data/dbfast2/seg2",
					"/data/dbfast2/seg4",
				}},
				)).Return(&idl.DeleteDataDirectoriesReply{}, nil)

			standbyClient := mock_idl.NewMockAgentClient(ctrl)
			// NOTE: we expect no call to the standby

			agentConns := []*idl.Connection{
				{AgentClient: sdw1Client, Hostname: "sdw1"},
				{AgentClient: sdw2Client, Hostname: "sdw2"},
				{AgentClient: standbyClient, Hostname: "standby"},
			}

			intermediate := hub.MustCreateCluster(t, append(primarySegConfigs, greenplum.SegConfig{ContentID: -1, DbID: 0, Port: 25431, Hostname: "coordinator", DataDir: "/data/qddir", Role: greenplum.PrimaryRole}))

			err := hub.DeleteCoordinatorAndPrimaryDataDirectories(step.DevNullStream, agentConns, intermediate)
			if err != nil {
				t.Errorf("unexpected err %#v", err)
			}
		})

		t.Run("returns error on failure", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			sdw1Client := mock_idl.NewMockAgentClient(ctrl)
			sdw1Client.EXPECT().DeleteDataDirectories(
				gomock.Any(),
				gomock.Any(),
			).Return(&idl.DeleteDataDirectoriesReply{}, nil)

			expected := errors.New("permission denied")
			sdw2ClientFailed := mock_idl.NewMockAgentClient(ctrl)
			sdw2ClientFailed.EXPECT().DeleteDataDirectories(
				gomock.Any(),
				gomock.Any(),
			).Return(nil, expected)

			agentConns := []*idl.Connection{
				{AgentClient: sdw1Client, Hostname: "sdw1"},
				{AgentClient: sdw2ClientFailed, Hostname: "sdw2"},
			}

			intermediate := hub.MustCreateCluster(t, append(primarySegConfigs, greenplum.SegConfig{ContentID: -1, DbID: 0, Port: 25431, Hostname: "coordinator", DataDir: "/data/qddir", Role: greenplum.PrimaryRole}))

			err := hub.DeleteCoordinatorAndPrimaryDataDirectories(step.DevNullStream, agentConns, intermediate)

			if !errors.Is(err, expected) {
				t.Errorf("got error %#v, want %#v", err, expected)
			}
		})
	})
}

func TestDeleteTablespaceDirectories(t *testing.T) {
	target := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "coordinator", DataDir: "/data/qddir", Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: -1, Hostname: "standby", DataDir: "/data/standby", Role: greenplum.MirrorRole},
		{DbID: 2, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
		{DbID: 3, ContentID: 0, Hostname: "msdw1", DataDir: "/data/dbfast_mirror1/seg1", Role: greenplum.MirrorRole},
		{DbID: 4, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		{DbID: 5, ContentID: 1, Hostname: "msdw2", DataDir: "/data/dbfast_mirror2/seg2", Role: greenplum.MirrorRole},
	})
	target.Version = semver.MustParse("6.1.0")

	t.Run("deletes tablespace directories only on the coordinator", func(t *testing.T) {
		tsDir1, _, tsLocation1 := testutils.MustMakeTablespaceDir(t, 16386)
		defer testutils.MustRemoveAll(t, tsLocation1)

		tsDir2, _, tsLocation2 := testutils.MustMakeTablespaceDir(t, 16387)
		defer testutils.MustRemoveAll(t, tsLocation2)

		systemTsDir, systemDbIdDir, systemTsLocation := testutils.MustMakeTablespaceDir(t, 1700)
		defer testutils.MustRemoveAll(t, systemTsLocation)

		coordinatorTablespaces := greenplum.SegmentTablespaces{
			16386: {
				Location:    tsLocation1,
				UserDefined: 1,
			},
			16387: {
				Location:    tsLocation2,
				UserDefined: 1,
			},
			1700: {
				Location:    systemTsLocation,
				UserDefined: 0,
			},
		}

		err := hub.DeleteTargetTablespacesOnCoordinator(step.DevNullStream, target, coordinatorTablespaces, "301908232")
		if err != nil {
			t.Errorf("DeleteTargetTablespacesOnCoordinator returned error %+v", err)
		}

		// verify user tablespace directories are deleted
		for _, dir := range []string{tsDir1, tsDir2} {

			testutils.PathMustNotExist(t, dir)

			dbIdDir := filepath.Dir(filepath.Clean(dir))
			testutils.PathMustNotExist(t, dbIdDir)
		}

		// verify system tablespace directories are not deleted
		testutils.PathMustExist(t, systemTsDir)
		testutils.PathMustExist(t, systemDbIdDir)

	})

	t.Run("deletes tablespace directories only on the primaries", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		tablespaces := map[int]greenplum.SegmentTablespaces{
			1: {
				16386: {
					Location:    "/tmp/testfs/coordinator/demoDataDir-1/16386",
					UserDefined: 1,
				},
				16387: {
					Location:    "/tmp/testfs/coordinator/demoDataDir-1/16387",
					UserDefined: 1,
				},
				1663: {
					// system tablespace locations do not include the tablespace oid
					Location:    "/data/qddir/demoDataDir-1",
					UserDefined: 0,
				},
			},
			2: {
				16386: {
					Location:    "/tmp/testfs/primary1/dbfast1/16386",
					UserDefined: 1,
				},
				16387: {
					Location:    "/tmp/testfs/primary1/dbfast1/16387",
					UserDefined: 1,
				},
				1663: {
					// system tablespace locations do not include the tablespace oid
					Location:    "/data/dbfast1/seg1",
					UserDefined: 0,
				},
			},
			4: {
				16386: {
					Location:    "/tmp/testfs/primary2/dbfast2/16386",
					UserDefined: 1,
				},
				16387: {
					Location:    "/tmp/testfs/primary2/dbfast2/16387",
					UserDefined: 1,
				},
				1663: {
					// system tablespace locations do not include the tablespace oid
					Location:    "/data/dbfast2/seg2",
					UserDefined: 0,
				},
			},
		}

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().DeleteTablespaceDirectories(
			gomock.Any(),
			equivalentTablespaceRequest(&idl.DeleteTablespaceRequest{
				Dirs: []string{
					"/tmp/testfs/primary1/dbfast1/16386/2/GPDB_6_301908232",
					"/tmp/testfs/primary1/dbfast1/16387/2/GPDB_6_301908232",
				}}),
		).Return(&idl.DeleteTablespaceReply{}, nil)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().DeleteTablespaceDirectories(
			gomock.Any(),
			equivalentTablespaceRequest(&idl.DeleteTablespaceRequest{
				Dirs: []string{
					"/tmp/testfs/primary2/dbfast2/16386/4/GPDB_6_301908232",
					"/tmp/testfs/primary2/dbfast2/16387/4/GPDB_6_301908232",
				}}),
		).Return(&idl.DeleteTablespaceReply{}, nil)

		coordinator := mock_idl.NewMockAgentClient(ctrl)
		standby := mock_idl.NewMockAgentClient(ctrl)

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
			{AgentClient: coordinator, Hostname: "coordinator"},
			{AgentClient: standby, Hostname: "standby"},
		}

		err := hub.DeleteTargetTablespacesOnPrimaries(agentConns, target, tablespaces, "301908232")
		if err != nil {
			t.Errorf("DeleteTargetTablespacesOnPrimaries returned error %+v", err)
		}
	})

	t.Run("errors when failing to delete tablespace directories on the primaries", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().DeleteTablespaceDirectories(
			gomock.Any(),
			gomock.Any(),
		).Return(&idl.DeleteTablespaceReply{}, nil)

		expected := errors.New("permission denied")
		failedClient := mock_idl.NewMockAgentClient(ctrl)
		failedClient.EXPECT().DeleteTablespaceDirectories(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: failedClient, Hostname: "sdw2"},
		}

		err := hub.DeleteTargetTablespacesOnPrimaries(agentConns, target, nil, "")

		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})

	t.Run("must not error out when target is not yet created", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw2 := mock_idl.NewMockAgentClient(ctrl)

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err := hub.DeleteTargetTablespacesOnPrimaries(agentConns, nil, nil, "")
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}
	})
}

// equivalentDeleteDataDirsRequest is a Matcher that can handle differences in order between
// two instances of DeleteTablespaceRequest.Dirs
func equivalentDeleteDataDirsRequest(req *idl.DeleteDataDirectoriesRequest) gomock.Matcher {
	return reqDeleteDataDirsMatcher{req}
}

type reqDeleteDataDirsMatcher struct {
	expected *idl.DeleteDataDirectoriesRequest
}

func (r reqDeleteDataDirsMatcher) Matches(x interface{}) bool {
	actual, ok := x.(*idl.DeleteDataDirectoriesRequest)
	if !ok {
		return false
	}

	// The key here is that Datadirs can be in any order. Sort them before
	// comparison.
	sort.Strings(r.expected.GetDatadirs())
	sort.Strings(actual.GetDatadirs())

	return reflect.DeepEqual(r.expected, actual)
}

func (r reqDeleteDataDirsMatcher) String() string {
	return fmt.Sprintf("is equivalent to %v", r.expected)
}

// equivalentTablespaceRequest is a Matcher that can handle differences in order between
// two instances of DeleteTablespaceRequest.Dirs
func equivalentTablespaceRequest(req *idl.DeleteTablespaceRequest) gomock.Matcher {
	return reqTablespaceMatcher{req}
}

type reqTablespaceMatcher struct {
	expected *idl.DeleteTablespaceRequest
}

func (r reqTablespaceMatcher) Matches(x interface{}) bool {
	actual, ok := x.(*idl.DeleteTablespaceRequest)
	if !ok {
		return false
	}

	// The key here is that Datadirs can be in any order. Sort them before
	// comparison.
	sort.Strings(r.expected.Dirs)
	sort.Strings(actual.Dirs)

	return reflect.DeepEqual(r.expected, actual)
}

func (r reqTablespaceMatcher) String() string {
	return fmt.Sprintf("is equivalent to %v", r.expected)
}
