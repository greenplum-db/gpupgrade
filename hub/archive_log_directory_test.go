// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

func TestArchiveLogDirectories(t *testing.T) {
	testlog.SetupTestLogger()

	var upgradeID upgrade.ID
	const targetCoordinatorHost = "cdw"

	t.Run("archive log directory succeeds", func(t *testing.T) {
		logDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, logDir)

		logArchiveDir := hub.GetLogArchiveDir(logDir, upgradeID, time.Now())
		defer testutils.MustRemoveAll(t, logArchiveDir)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sdwClient := mock_idl.NewMockAgentClient(ctrl)
		sdwClient.EXPECT().ArchiveLogDirectory(
			gomock.Any(),
			&idl.ArchiveLogDirectoryRequest{LogArchiveDir: logArchiveDir},
		).Return(&idl.ArchiveLogDirectoryReply{}, nil).Times(1)

		agentConns := []*idl.Connection{
			{AgentClient: sdwClient, Hostname: "sdw"},
		}

		err := hub.ArchiveLogDirectories(logDir, logArchiveDir, agentConns, targetCoordinatorHost)
		if err != nil {
			t.Errorf("unexpected err %+v", err)
		}

		testutils.PathMustNotExist(t, logDir)
		testutils.PathMustExist(t, logArchiveDir)
	})

	t.Run("archive log directory is idempotent with different timestamp each time", func(t *testing.T) {
		logDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, logDir)

		logArchiveDir := hub.GetLogArchiveDir(logDir, upgradeID, time.Now())
		defer testutils.MustRemoveAll(t, logArchiveDir)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sdwClient := mock_idl.NewMockAgentClient(ctrl)
		sdwClient.EXPECT().ArchiveLogDirectory(
			gomock.Any(),
			&idl.ArchiveLogDirectoryRequest{LogArchiveDir: logArchiveDir},
		).Return(&idl.ArchiveLogDirectoryReply{}, nil).Times(1)

		agentConns := []*idl.Connection{
			{AgentClient: sdwClient, Hostname: "sdw"},
		}

		err := hub.ArchiveLogDirectories(logDir, logArchiveDir, agentConns, targetCoordinatorHost)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		testutils.PathMustNotExist(t, logDir)
		testutils.PathMustExist(t, logArchiveDir)

		logDir = testutils.GetTempDir(t, "") // re-create logDir which gets created when Cli, Hub, or Agent process starts
		defer testutils.MustRemoveAll(t, logDir)
		logArchiveDir = hub.GetLogArchiveDir(logDir, upgradeID, time.Now())
		defer testutils.MustRemoveAll(t, logArchiveDir)

		sdwClient = mock_idl.NewMockAgentClient(ctrl)
		sdwClient.EXPECT().ArchiveLogDirectory(
			gomock.Any(),
			&idl.ArchiveLogDirectoryRequest{LogArchiveDir: logArchiveDir},
		).Return(&idl.ArchiveLogDirectoryReply{}, nil).Times(1)

		agentConns = []*idl.Connection{
			{AgentClient: sdwClient, Hostname: "sdw"},
		}

		err = hub.ArchiveLogDirectories(logDir, logArchiveDir, agentConns, targetCoordinatorHost)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		testutils.PathMustNotExist(t, logDir)
		testutils.PathMustExist(t, logArchiveDir)

		logDir = testutils.GetTempDir(t, "") // re-create logDir which gets created when Cli, Hub, or Agent process starts
		defer testutils.MustRemoveAll(t, logDir)
		logArchiveDir = hub.GetLogArchiveDir(logDir, upgradeID, time.Now())
		defer testutils.MustRemoveAll(t, logArchiveDir)

		sdwClient = mock_idl.NewMockAgentClient(ctrl)
		sdwClient.EXPECT().ArchiveLogDirectory(
			gomock.Any(),
			&idl.ArchiveLogDirectoryRequest{LogArchiveDir: logArchiveDir},
		).Return(&idl.ArchiveLogDirectoryReply{}, nil).Times(1)

		agentConns = []*idl.Connection{
			{AgentClient: sdwClient, Hostname: "sdw"},
		}

		err = hub.ArchiveLogDirectories(logDir, logArchiveDir, agentConns, targetCoordinatorHost)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		testutils.PathMustNotExist(t, logDir)
		testutils.PathMustExist(t, logArchiveDir)
	})
}

func TestArchiveSegmentLogDirectories(t *testing.T) {
	var upgradeID upgrade.ID
	const targetCoordinatorHost = "cdw"

	t.Run("archive segment log directories", func(t *testing.T) {
		logDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, logDir)

		logArchiveDir := hub.GetLogArchiveDir(logDir, upgradeID, time.Now())
		defer testutils.MustRemoveAll(t, logArchiveDir)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sdwClient := mock_idl.NewMockAgentClient(ctrl)
		sdwClient.EXPECT().ArchiveLogDirectory(
			gomock.Any(),
			&idl.ArchiveLogDirectoryRequest{LogArchiveDir: logArchiveDir},
		).Return(&idl.ArchiveLogDirectoryReply{}, nil).Times(1)

		agentConns := []*idl.Connection{
			{AgentClient: sdwClient, Hostname: "sdw"},
		}

		err := hub.ArchiveSegmentLogDirectories(agentConns, targetCoordinatorHost, logArchiveDir)
		if err != nil {
			t.Errorf("unexpected err %+v", err)
		}
	})

	t.Run("bubbles up errors", func(t *testing.T) {
		logDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, logDir)

		logArchiveDir := hub.GetLogArchiveDir(logDir, upgradeID, time.Now())
		defer testutils.MustRemoveAll(t, logArchiveDir)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		expected := errors.New("permission denied")
		failedClient := mock_idl.NewMockAgentClient(ctrl)
		failedClient.EXPECT().ArchiveLogDirectory(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected).Times(1)

		agentConns := []*idl.Connection{
			{AgentClient: failedClient, Hostname: "sdw"},
		}

		err := hub.ArchiveSegmentLogDirectories(agentConns, targetCoordinatorHost, logArchiveDir)
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})
}

func TestGetLogArchiveDir(t *testing.T) {
	// Make sure every part of the date is distinct, to catch mistakes in formatting (e.g. using seconds rather than minutes).
	timeStamp := time.Date(2000, 03, 14, 12, 15, 45, 1, time.Local)

	var upgradeID upgrade.ID
	actual := hub.GetLogArchiveDir("/tmp/log/dir", upgradeID, timeStamp)

	expected := fmt.Sprintf("/tmp/log/gpupgrade-%s-20000314T121509", upgradeID.String())
	if actual != expected {
		t.Errorf("got %q want %q", actual, expected)
	}
}
