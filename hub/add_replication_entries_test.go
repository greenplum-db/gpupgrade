// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"net"
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

func TestAddReplicationEntriesOnPrimaries(t *testing.T) {
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

	t.Run("succeeds when useHbaHostnames is false", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().AddReplicationEntries(
			gomock.Any(),
			&idl.AddReplicationEntriesRequest{
				Entries: []*idl.AddReplicationEntriesRequest_Entry{
					{
						DataDir:   "/data/dbfast1/seg.HqtFHX54y0o.1",
						User:      "gpadmin",
						HostAddrs: []string{"sdw2"},
					}},
			},
		).Return(&idl.AddReplicationEntriesReply{}, nil)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().AddReplicationEntries(
			gomock.Any(),
			&idl.AddReplicationEntriesRequest{
				Entries: []*idl.AddReplicationEntriesRequest_Entry{
					{
						DataDir:   "/data/dbfast2/seg.HqtFHX54y0o.2",
						User:      "gpadmin",
						HostAddrs: []string{"sdw1"},
					}},
			},
		).Return(&idl.AddReplicationEntriesReply{}, nil)

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err := hub.AddReplicationEntriesOnPrimaries(agentConns, intermediate, false)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("succeeds when useHbaHostnames is true", func(t *testing.T) {
		utils.System.LookupIP = func(host string) ([]net.IP, error) {
			if host == "sdw2" {
				return []net.IP{net.ParseIP("10.0.0.1"), net.ParseIP("10.0.0.2")}, nil
			}

			return []net.IP{net.ParseIP("FE80::903A:1C1A:E802:11E4"), net.ParseIP("FE80::903A:1C1A:E802:11E5")}, nil
		}
		defer func() {
			utils.System.LookupIP = net.LookupIP
		}()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().AddReplicationEntries(
			gomock.Any(),
			&idl.AddReplicationEntriesRequest{
				Entries: []*idl.AddReplicationEntriesRequest_Entry{
					{
						DataDir:   "/data/dbfast1/seg.HqtFHX54y0o.1",
						User:      "gpadmin",
						HostAddrs: []string{"10.0.0.1/32", "10.0.0.2/32"},
					}},
			},
		).Return(&idl.AddReplicationEntriesReply{}, nil)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().AddReplicationEntries(
			gomock.Any(),
			&idl.AddReplicationEntriesRequest{
				Entries: []*idl.AddReplicationEntriesRequest_Entry{
					{
						DataDir:   "/data/dbfast2/seg.HqtFHX54y0o.2",
						User:      "gpadmin",
						HostAddrs: []string{"fe80::903a:1c1a:e802:11e4/128", "fe80::903a:1c1a:e802:11e5/128"},
					}},
			},
		).Return(&idl.AddReplicationEntriesReply{}, nil)

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err := hub.AddReplicationEntriesOnPrimaries(agentConns, intermediate, true)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("returns errors when failing on segments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		expected := errors.New("permission denied")
		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().AddReplicationEntries(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().AddReplicationEntries(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err := hub.AddReplicationEntriesOnPrimaries(agentConns, intermediate, false)
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

		err := hub.AddReplicationEntriesOnPrimaries(nil, intermediate, true)
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})

	t.Run("returns errors when failing to get ip addresses", func(t *testing.T) {
		expected := errors.New("connection failed")
		utils.System.LookupIP = func(host string) ([]net.IP, error) {
			return nil, expected
		}
		defer func() {
			utils.System.LookupIP = net.LookupIP
		}()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		agentConns := []*idl.Connection{
			{AgentClient: nil, Hostname: "sdw1"},
			{AgentClient: nil, Hostname: "sdw2"},
		}

		err := hub.AddReplicationEntriesOnPrimaries(agentConns, intermediate, true)
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
}
