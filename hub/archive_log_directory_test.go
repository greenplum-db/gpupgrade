// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/go-multierror"

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

		agentConns := []*hub.Connection{
			{nil, sdwClient, "sdw", nil},
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

		agentConns := []*hub.Connection{
			{nil, failedClient, "sdw", nil},
		}

		err := hub.ArchiveSegmentLogDirectories(agentConns, "", newDir)
		var multiErr *multierror.Error
		if !errors.As(err, &multiErr) {
			t.Fatalf("got error %#v, want type %T", err, multiErr)
		}

		if len(multiErr.Errors) != 1 {
			t.Errorf("received %d errors, want %d", len(multiErr.Errors), 1)
		}

		for _, err := range multiErr.Errors {
			if !errors.Is(err, expected) {
				t.Errorf("got error %#v, want %#v", expected, err)
			}
		}
	})
}
