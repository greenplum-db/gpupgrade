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

func TestDeleteStateDirectories(t *testing.T) {
	testlog.SetupLogger()

	t.Run("DeleteStateDirectories", func(t *testing.T) {
		t.Run("deletes state directories on all hosts except for the host that gets passed in", func(t *testing.T) {
			excludeHostname := "master-host"
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			sdw1Client := mock_idl.NewMockAgentClient(ctrl)
			sdw1Client.EXPECT().DeleteStateDirectory(
				gomock.Any(),
				&idl.DeleteStateDirectoryRequest{},
			).Return(&idl.DeleteStateDirectoryReply{}, nil)

			standbyClient := mock_idl.NewMockAgentClient(ctrl)
			standbyClient.EXPECT().DeleteStateDirectory(
				gomock.Any(),
				&idl.DeleteStateDirectoryRequest{},
			).Return(&idl.DeleteStateDirectoryReply{}, nil)

			masterHostClient := mock_idl.NewMockAgentClient(ctrl)
			// NOTE: we expect no call to the master

			agentConns := []*idl.Connection{
				{AgentClient: sdw1Client, Hostname: "sdw1"},
				{AgentClient: standbyClient, Hostname: "standby"},
				{AgentClient: masterHostClient, Hostname: excludeHostname},
			}

			err := hub.DeleteStateDirectories(agentConns, excludeHostname)
			if err != nil {
				t.Errorf("unexpected err %#v", err)
			}
		})

		t.Run("returns error on failure", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			sdw1Client := mock_idl.NewMockAgentClient(ctrl)
			sdw1Client.EXPECT().DeleteStateDirectory(
				gomock.Any(),
				gomock.Any(),
			).Return(&idl.DeleteStateDirectoryReply{}, nil)

			expected := errors.New("permission denied")
			sdw2ClientFailed := mock_idl.NewMockAgentClient(ctrl)
			sdw2ClientFailed.EXPECT().DeleteStateDirectory(
				gomock.Any(),
				gomock.Any(),
			).Return(nil, expected)

			agentConns := []*idl.Connection{
				{AgentClient: sdw1Client, Hostname: "sdw1"},
				{AgentClient: sdw2ClientFailed, Hostname: "sdw2"},
			}

			err := hub.DeleteStateDirectories(agentConns, "")

			if !errors.Is(err, expected) {
				t.Errorf("got error %#v, want %#v", err, expected)
			}
		})
	})
}
