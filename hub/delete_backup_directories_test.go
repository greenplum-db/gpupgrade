// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"os"
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/greenplum-db/gpupgrade/config/backupdir"
	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func TestDeleteBackupDirectories(t *testing.T) {
	source := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "coordinator", DataDir: "/data/coordinator/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby/seg-1", Port: 16432, Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data1/primaries/seg1", Port: 25433, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data2/mirrors/seg1", Port: 25434, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data2/primaries/seg2", Port: 25435, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data1/mirrors/seg2", Port: 25436, Role: greenplum.MirrorRole},
		{DbID: 7, ContentID: 2, Hostname: "sdw3", DataDir: "/data3/mirrors/seg3", Port: 25437, Role: greenplum.MirrorRole},
		{DbID: 8, ContentID: 2, Hostname: "sdw3", DataDir: "/data3/primaries/seg3", Port: 25438, Role: greenplum.PrimaryRole},
	})

	backupDirs, err := backupdir.ParseParentBackupDirs("", *source)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("succeeds in deleting backup directory on coordinator", func(t *testing.T) {
		coordinatorBackupDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, coordinatorBackupDir)

		backupDirs := backupdir.BackupDirs{}
		backupDirs.CoordinatorBackupDir = coordinatorBackupDir

		err := hub.DeleteBackupDirectories(step.DevNullStream, nil, backupDirs)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		testutils.PathMustNotExist(t, coordinatorBackupDir)
	})

	t.Run("errors when failing to delete coordinator backup directory", func(t *testing.T) {
		expected := os.ErrInvalid
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			return nil, expected
		}
		defer utils.ResetSystemFunctions()

		err := hub.DeleteBackupDirectories(step.DevNullStream, nil, backupdir.BackupDirs{})
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v want %#v", err, expected)
		}
	})

	t.Run("only makes requests to hosts in the backup directories which does not include the standby", func(t *testing.T) {
		coordinatorBackupDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, coordinatorBackupDir)

		backupDirs.CoordinatorBackupDir = coordinatorBackupDir

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().DeleteBackupDirectory(
			gomock.Any(),
			&idl.DeleteBackupDirectoryRequest{
				BackupDir: backupDirs.AgentHostsToBackupDir["sdw1"],
			},
		).Return(&idl.DeleteBackupDirectoryReply{}, nil)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().DeleteBackupDirectory(
			gomock.Any(),
			&idl.DeleteBackupDirectoryRequest{
				BackupDir: backupDirs.AgentHostsToBackupDir["sdw2"],
			},
		).Return(&idl.DeleteBackupDirectoryReply{}, nil)

		agentConns := []*idl.Connection{
			{AgentClient: mock_idl.NewMockAgentClient(ctrl), Hostname: "standby"},
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err := hub.DeleteBackupDirectories(step.DevNullStream, agentConns, backupDirs)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("errors when failing to delete backup directories on segments", func(t *testing.T) {
		coordinatorBackupDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, coordinatorBackupDir)

		backupDirs.CoordinatorBackupDir = coordinatorBackupDir

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().DeleteBackupDirectory(
			gomock.Any(),
			&idl.DeleteBackupDirectoryRequest{
				BackupDir: backupDirs.AgentHostsToBackupDir["sdw1"],
			},
		).Return(&idl.DeleteBackupDirectoryReply{}, nil)

		expected := errors.New("permission denied")
		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().DeleteBackupDirectory(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		sdw3 := mock_idl.NewMockAgentClient(ctrl)
		sdw3.EXPECT().DeleteBackupDirectory(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		agentConns := []*idl.Connection{
			{AgentClient: mock_idl.NewMockAgentClient(ctrl), Hostname: "standby"},
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
			{AgentClient: sdw3, Hostname: "sdw3"},
		}

		err := hub.DeleteBackupDirectories(step.DevNullStream, agentConns, backupDirs)
		var errs errorlist.Errors
		if !errors.As(err, &errs) {
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
