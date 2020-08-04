// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
)

func TestDeleteSegmentDataDirs(t *testing.T) {
	segConfigs := []greenplum.SegConfig{
		{ContentID: -1, DbID: 0, Port: 25431, Hostname: "master", DataDir: "/data/qddir", Role: greenplum.PrimaryRole},
		{ContentID: -1, DbID: 1, Port: 25431, Hostname: "standby", DataDir: "/data/standby", Role: greenplum.MirrorRole},
	}

	primarySegConfigs := []greenplum.SegConfig{
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		{ContentID: 2, DbID: 4, Port: 25434, Hostname: "sdw1", DataDir: "/data/dbfast1/seg3", Role: greenplum.PrimaryRole},
		{ContentID: 3, DbID: 5, Port: 25435, Hostname: "sdw2", DataDir: "/data/dbfast2/seg4", Role: greenplum.PrimaryRole},
	}
	segConfigs = append(segConfigs, primarySegConfigs...)

	mirrorSegConfigs := []greenplum.SegConfig{
		{ContentID: 0, DbID: 6, Port: 35432, Hostname: "sdw1", DataDir: "/data/dbfast_mirror1/seg1", Role: greenplum.MirrorRole},
		{ContentID: 1, DbID: 7, Port: 35433, Hostname: "sdw2", DataDir: "/data/dbfast_mirror2/seg2", Role: greenplum.MirrorRole},
		{ContentID: 2, DbID: 8, Port: 35434, Hostname: "sdw1", DataDir: "/data/dbfast_mirror1/seg3", Role: greenplum.MirrorRole},
		{ContentID: 3, DbID: 9, Port: 35435, Hostname: "sdw2", DataDir: "/data/dbfast_mirror2/seg4", Role: greenplum.MirrorRole},
	}
	segConfigs = append(segConfigs, mirrorSegConfigs...)

	c := hub.MustCreateCluster(t, segConfigs)

	testhelper.SetupTestLogger()

	t.Run("DeleteMirrorAndStandbyDataDirectories", func(t *testing.T) {
		t.Run("deletes standby and mirror data directories", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			sdw1Client := mock_idl.NewMockAgentClient(ctrl)
			sdw1Client.EXPECT().DeleteDataDirectories(
				gomock.Any(),
				&idl.DeleteDataDirectoriesRequest{Datadirs: []string{
					"/data/dbfast_mirror1/seg1",
					"/data/dbfast_mirror1/seg3",
				}},
			).Return(&idl.DeleteDataDirectoriesReply{}, nil)

			sdw2Client := mock_idl.NewMockAgentClient(ctrl)
			sdw2Client.EXPECT().DeleteDataDirectories(
				gomock.Any(),
				&idl.DeleteDataDirectoriesRequest{Datadirs: []string{
					"/data/dbfast_mirror2/seg2",
					"/data/dbfast_mirror2/seg4",
				}},
			).Return(&idl.DeleteDataDirectoriesReply{}, nil)

			standbyClient := mock_idl.NewMockAgentClient(ctrl)
			standbyClient.EXPECT().DeleteDataDirectories(
				gomock.Any(),
				&idl.DeleteDataDirectoriesRequest{Datadirs: []string{"/data/standby"}},
			).Return(&idl.DeleteDataDirectoriesReply{}, nil)

			agentConns := []*hub.Connection{
				{nil, sdw1Client, "sdw1", nil},
				{nil, sdw2Client, "sdw2", nil},
				{nil, standbyClient, "standby", nil},
			}

			err := hub.DeleteMirrorAndStandbyDataDirectories(agentConns, c)
			if err != nil {
				t.Errorf("unexpected err %#v", err)
			}
		})
	})

	t.Run("DeletePrimaryDataDirectories", func(t *testing.T) {
		t.Run("deletes primary data directories", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			sdw1Client := mock_idl.NewMockAgentClient(ctrl)
			sdw1Client.EXPECT().DeleteDataDirectories(
				gomock.Any(),
				&idl.DeleteDataDirectoriesRequest{Datadirs: []string{
					"/data/dbfast1/seg1",
					"/data/dbfast1/seg3",
				}},
			).Return(&idl.DeleteDataDirectoriesReply{}, nil)

			sdw2Client := mock_idl.NewMockAgentClient(ctrl)
			sdw2Client.EXPECT().DeleteDataDirectories(
				gomock.Any(),
				&idl.DeleteDataDirectoriesRequest{Datadirs: []string{
					"/data/dbfast2/seg2",
					"/data/dbfast2/seg4",
				}},
			).Return(&idl.DeleteDataDirectoriesReply{}, nil)

			standbyClient := mock_idl.NewMockAgentClient(ctrl)
			// NOTE: we expect no call to the standby

			agentConns := []*hub.Connection{
				{nil, sdw1Client, "sdw1", nil},
				{nil, sdw2Client, "sdw2", nil},
				{nil, standbyClient, "standby", nil},
			}

			err := hub.DeletePrimaryDataDirectories(agentConns, primarySegConfigs)
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

			agentConns := []*hub.Connection{
				{nil, sdw1Client, "sdw1", nil},
				{nil, sdw2ClientFailed, "sdw2", nil},
			}

			err := hub.DeletePrimaryDataDirectories(agentConns, primarySegConfigs)

			var multiErr *multierror.Error
			if !xerrors.As(err, &multiErr) {
				t.Fatalf("got error %#v, want type %T", err, multiErr)
			}

			if len(multiErr.Errors) != 1 {
				t.Errorf("received %d errors, want %d", len(multiErr.Errors), 1)
			}

			for _, err := range multiErr.Errors {
				if !xerrors.Is(err, expected) {
					t.Errorf("got error %#v, want %#v", expected, err)
				}
			}
		})
	})
}
