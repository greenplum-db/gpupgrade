// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
)

const newDir = "NewDirectory"

func TestArchiveLogDirectories(t *testing.T) {
	testlog.SetupLogger()

	t.Run("archive segment log directories", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sdwClient := mock_idl.NewMockAgentClient(ctrl)
		sdwClient.EXPECT().ArchiveLogDirectory(
			gomock.Any(),
			&idl.ArchiveLogDirectoryRequest{NewDir: newDir},
		).Return(&idl.ArchiveLogDirectoryReply{}, nil).Times(1)

		agentConns := []*idl.Connection{
			{AgentClient: sdwClient, Hostname: "sdw"},
		}

		err := hub.ArchiveSegmentLogDirectories(agentConns, "", newDir)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("bubbles up errors", func(t *testing.T) {
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

		err := hub.ArchiveSegmentLogDirectories(agentConns, "", newDir)
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})
}
