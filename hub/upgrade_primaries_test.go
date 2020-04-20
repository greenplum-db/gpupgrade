// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
)

func TestUpgradePrimaries(t *testing.T) {
	source := hub.MustCreateCluster(t, []greenplum.SegConfig{
		{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
		{ContentID: 1, DbID: 3, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
	})
	source.BinDir = "/usr/local/greenplum-db"
	source.Version = dbconn.NewVersion("5.0.0")

	target := hub.MustCreateCluster(t, []greenplum.SegConfig{
		{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1_upgrade/seg1", Role: greenplum.PrimaryRole},
		{ContentID: 1, DbID: 3, Hostname: "sdw2", DataDir: "/data/dbfast2_upgrade/seg2", Role: greenplum.PrimaryRole},
	})
	target.BinDir = "/usr/local/greenplum-db-new"
	target.Version = dbconn.NewVersion("6.0.0")

	pairs := map[string][]*idl.DataDirPair{
		"sdw1": {
			{
				SourceDataDir: "/data/dbfast1",
				TargetDataDir: "/data/dbfast1_upgrade",
				SourcePort:    15432,
				TargetPort:    15433,
				Content:       0,
				DBID:          2,
			},
		},
		"sdw2": {
			{
				SourceDataDir: "/data/dbfast2",
				TargetDataDir: "/data/dbfast2_upgrade",
				SourcePort:    15432,
				TargetPort:    15433,
				Content:       1,
				DBID:          3,
			},
		},
	}

	t.Run("sends expected request when upgrading primaries", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client1 := mock_idl.NewMockAgentClient(ctrl)
		client1.EXPECT().UpgradePrimaries(
			gomock.Any(),
			&idl.UpgradePrimariesRequest{
				SourceBinDir:    "/usr/local/greenplum-db",
				TargetBinDir:    "/usr/local/greenplum-db-new",
				TargetVersion:   dbconn.NewVersion("6.0.0").VersionString,
				DataDirPairs:    pairs["sdw1"],
				CheckOnly:       false,
				UseLinkMode:     false,
				MasterBackupDir: "",
			},
		).Return(&idl.UpgradePrimariesReply{}, nil)

		client2 := mock_idl.NewMockAgentClient(ctrl)
		client2.EXPECT().UpgradePrimaries(
			gomock.Any(),
			&idl.UpgradePrimariesRequest{
				SourceBinDir:    "/usr/local/greenplum-db",
				TargetBinDir:    "/usr/local/greenplum-db-new",
				TargetVersion:   dbconn.NewVersion("6.0.0").VersionString,
				DataDirPairs:    pairs["sdw2"],
				CheckOnly:       false,
				UseLinkMode:     false,
				MasterBackupDir: "",
			},
		).Return(&idl.UpgradePrimariesReply{}, nil)

		agentConns := []*hub.Connection{
			{nil, client1, "sdw1", nil},
			{nil, client2, "sdw2", nil},
		}

		err := hub.UpgradePrimaries(hub.UpgradePrimaryArgs{
			CheckOnly:       false,
			MasterBackupDir: "",
			AgentConns:      agentConns,
			DataDirPairMap:  pairs,
			Source:          source,
			Target:          target,
			UseLinkMode:     false,
		})
		if err != nil {
			t.Errorf("got unexpected error: %+v", err)
		}
	})

	t.Run("errors when upgrading primary fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client1 := mock_idl.NewMockAgentClient(ctrl)
		client1.EXPECT().UpgradePrimaries(
			gomock.Any(),
			&idl.UpgradePrimariesRequest{
				SourceBinDir:    "/usr/local/greenplum-db",
				TargetBinDir:    "/usr/local/greenplum-db-new",
				TargetVersion:   dbconn.NewVersion("6.0.0").VersionString,
				DataDirPairs:    pairs["sdw1"],
				CheckOnly:       false,
				UseLinkMode:     false,
				MasterBackupDir: "",
			},
		).Return(&idl.UpgradePrimariesReply{}, nil)

		expected := errors.New("permission denied")
		failedClient := mock_idl.NewMockAgentClient(ctrl)
		failedClient.EXPECT().UpgradePrimaries(
			gomock.Any(),
			&idl.UpgradePrimariesRequest{
				SourceBinDir:    "/usr/local/greenplum-db",
				TargetBinDir:    "/usr/local/greenplum-db-new",
				TargetVersion:   dbconn.NewVersion("6.0.0").VersionString,
				DataDirPairs:    pairs["sdw2"],
				CheckOnly:       false,
				UseLinkMode:     false,
				MasterBackupDir: "",
			},
		).Return(&idl.UpgradePrimariesReply{}, expected)

		agentConns := []*hub.Connection{
			{nil, client1, "sdw1", nil},
			{nil, failedClient, "sdw2", nil},
		}

		err := hub.UpgradePrimaries(hub.UpgradePrimaryArgs{
			CheckOnly:       false,
			MasterBackupDir: "",
			AgentConns:      agentConns,
			DataDirPairMap:  pairs,
			Source:          source,
			Target:          target,
			UseLinkMode:     false,
		})
		if err == nil {
			t.Fatal("expected error got nil")
		}

		// XXX it'd be nice if we didn't couple against a hardcoded string here,
		// but it's difficult to unwrap multierror with the new xerrors interface.
		if !strings.Contains(err.Error(), "failed to upgrade primary segment on host sdw2") ||
			!strings.Contains(err.Error(), expected.Error()) {
			t.Errorf("error %q did not contain expected contents '%q'", err.Error(), expected.Error())
		}
	})
}

func TestGetDataDirPairs(t *testing.T) {
	t.Run("errors if source and target clusters have different number of segments", func(t *testing.T) {
		source := hub.MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
			{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
			{ContentID: 1, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		})

		target := hub.MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
		})

		conf := &hub.Config{
			Source: source,
			Target: target,
		}
		server := hub.New(conf, nil, "")

		_, err := server.GetDataDirPairs()
		if !xerrors.Is(err, hub.ErrInvalidCluster) {
			t.Errorf("returned error %#v got: %#v", err, hub.ErrInvalidCluster)
		}
	})

	t.Run("errors if source and target clusters have different content ids", func(t *testing.T) {
		source := hub.MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
			{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
			{ContentID: 1, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		})

		target := hub.MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
			{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
			{ContentID: 2, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		})

		conf := &hub.Config{
			Source: source,
			Target: target,
		}
		server := hub.New(conf, nil, "")

		_, err := server.GetDataDirPairs()
		if !xerrors.Is(err, hub.ErrInvalidCluster) {
			t.Errorf("returned error %#v got: %#v", err, hub.ErrInvalidCluster)
		}
	})

	t.Run("errors if source and target cluster hostnames differ", func(t *testing.T) {
		source := hub.MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
			{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
			{ContentID: 1, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		})

		target := hub.MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
			{ContentID: 0, DbID: 2, Hostname: "localhost", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
			{ContentID: 1, DbID: 3, Hostname: "localhost", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		})

		conf := &hub.Config{
			Source: source,
			Target: target,
		}
		server := hub.New(conf, nil, "")

		_, err := server.GetDataDirPairs()
		if !xerrors.Is(err, hub.ErrInvalidCluster) {
			t.Errorf("returned error %#v got: %#v", err, hub.ErrInvalidCluster)
		}
	})
}
