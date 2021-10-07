//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"os/user"
	"testing"

	"github.com/golang/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func TestCreateRecoveryConfOnSegments(t *testing.T) {
	intermediate := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "master", DataDir: "/data/qddir/seg.HqtFHX54y0o.-1", Port: 50432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby.HqtFHX54y0o", Port: 50433, Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg.HqtFHX54y0o.1", Port: 50434, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg.HqtFHX54y0o.1", Port: 50435, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg.HqtFHX54y0o.2", Port: 50436, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg.HqtFHX54y0o.2", Port: 50437, Role: greenplum.MirrorRole},
	})

	utils.System.Current = func() (*user.User, error) {
		return &user.User{Username: "gpadmin"}, nil
	}
	defer func() {
		utils.System.Current = user.Current
	}()

	t.Run("succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().CreateRecoveryConf(
			gomock.Any(),
			&idl.CreateRecoveryConfRequest{
				Connections: []*idl.CreateRecoveryConfRequest_Connection{
					{
						MirrorDataDir: "/data/dbfast_mirror2/seg.HqtFHX54y0o.2",
						User:          "gpadmin",
						PrimaryHost:   "sdw2",
						PrimaryPort:   int32(50436),
					}},
			},
		).Return(&idl.CreateRecoveryConfReply{}, nil)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().CreateRecoveryConf(
			gomock.Any(),
			&idl.CreateRecoveryConfRequest{
				Connections: []*idl.CreateRecoveryConfRequest_Connection{
					{
						MirrorDataDir: "/data/dbfast_mirror1/seg.HqtFHX54y0o.1",
						User:          "gpadmin",
						PrimaryHost:   "sdw1",
						PrimaryPort:   int32(50434),
					}},
			},
		).Return(&idl.CreateRecoveryConfReply{}, nil)

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err := hub.CreateRecoveryConfOnSegments(agentConns, intermediate)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("returns errors when failing on segments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		expected := errors.New("permission denied")
		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().CreateRecoveryConf(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().CreateRecoveryConf(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err := hub.CreateRecoveryConfOnSegments(agentConns, intermediate)
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

	t.Run("returns errors when failing to get user", func(t *testing.T) {
		expected := errors.New("connection failed")
		utils.System.Current = func() (*user.User, error) {
			return nil, expected
		}
		defer func() {
			utils.System.Current = user.Current
		}()

		err := hub.CreateRecoveryConfOnSegments(nil, intermediate)
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})
}
