// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/golang/mock/gomock"

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
	source.GPHome = "/usr/local/greenplum-db"
	source.Version = semver.MustParse("5.0.0")

	target := hub.MustCreateCluster(t, []greenplum.SegConfig{
		{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1_upgrade/seg1", Role: greenplum.PrimaryRole},
		{ContentID: 1, DbID: 3, Hostname: "sdw2", DataDir: "/data/dbfast2_upgrade/seg2", Role: greenplum.PrimaryRole},
	})
	target.GPHome = "/usr/local/greenplum-db-new"
	target.Version = semver.MustParse("6.0.0")

	segmentDbId2Tablespaces := map[int32]*idl.TablespaceInfo{
		1663: &idl.TablespaceInfo{Name: "tblspc1", Location: "/tmp/primary1/1663", UserDefined: false},
		1664: &idl.TablespaceInfo{Name: "tblspc2", Location: "/tmp/primary1/1664", UserDefined: true}}

	segmentDbId3Tablespaces := map[int32]*idl.TablespaceInfo{
		1663: &idl.TablespaceInfo{Name: "tblspc1", Location: "/tmp/primary1/1663", UserDefined: false},
		1664: &idl.TablespaceInfo{Name: "tblspc2", Location: "/tmp/primary1/1664", UserDefined: true}}

	pairs := map[string][]*idl.DataDirPair{
		"sdw1": {
			{
				SourceDataDir: "/data/dbfast1",
				TargetDataDir: "/data/dbfast1_upgrade",
				SourcePort:    15432,
				TargetPort:    15433,
				Content:       0,
				DBID:          2,
				Tablespaces:   segmentDbId2Tablespaces,
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
				Tablespaces:   segmentDbId3Tablespaces,
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
				SourceBinDir:               "/usr/local/greenplum-db/bin",
				TargetBinDir:               "/usr/local/greenplum-db-new/bin",
				TargetVersion:              semver.MustParse("6.0.0").String(),
				DataDirPairs:               pairs["sdw1"],
				CheckOnly:                  false,
				UseLinkMode:                false,
				MasterBackupDir:            "",
				TablespacesMappingFilePath: "/tmp/tablespaces_mapping.txt",
			},
		).Return(&idl.UpgradePrimariesReply{}, nil)

		client2 := mock_idl.NewMockAgentClient(ctrl)
		client2.EXPECT().UpgradePrimaries(
			gomock.Any(),
			&idl.UpgradePrimariesRequest{
				SourceBinDir:               "/usr/local/greenplum-db/bin",
				TargetBinDir:               "/usr/local/greenplum-db-new/bin",
				TargetVersion:              semver.MustParse("6.0.0").String(),
				DataDirPairs:               pairs["sdw2"],
				CheckOnly:                  false,
				UseLinkMode:                false,
				MasterBackupDir:            "",
				TablespacesMappingFilePath: "/tmp/tablespaces_mapping.txt",
			},
		).Return(&idl.UpgradePrimariesReply{}, nil)

		agentConns := []*idl.Connection{
			{AgentClient: client1, Hostname: "sdw1"},
			{AgentClient: client2, Hostname: "sdw2"},
		}

		err := hub.UpgradePrimaries(hub.UpgradePrimaryArgs{
			CheckOnly:              false,
			MasterBackupDir:        "",
			AgentConns:             agentConns,
			DataDirPairMap:         pairs,
			Source:                 source,
			IntermediateTarget:     target,
			UseLinkMode:            false,
			TablespacesMappingFile: "/tmp/tablespaces_mapping.txt",
		})
		if err != nil {
			t.Errorf("got unexpected error: %+v", err)
		}
	})

	t.Run("errors when checking or upgrading primary fails", func(t *testing.T) {
		errCases := []struct {
			name         string
			CheckOnly    bool
			failedAction string
		}{
			{
				name:         "errors when upgrading primary fails",
				CheckOnly:    false,
				failedAction: "upgrade",
			},
			{
				name:         "errors when checking primary fails",
				CheckOnly:    true,
				failedAction: "check",
			},
		}

		for _, c := range errCases {
			t.Run(c.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				client1 := mock_idl.NewMockAgentClient(ctrl)
				client1.EXPECT().UpgradePrimaries(
					gomock.Any(),
					&idl.UpgradePrimariesRequest{
						SourceBinDir:    "/usr/local/greenplum-db/bin",
						TargetBinDir:    "/usr/local/greenplum-db-new/bin",
						TargetVersion:   semver.MustParse("6.0.0").String(),
						DataDirPairs:    pairs["sdw1"],
						CheckOnly:       c.CheckOnly,
						UseLinkMode:     false,
						MasterBackupDir: "",
					},
				).Return(&idl.UpgradePrimariesReply{}, nil)

				expected := errors.New("permission denied")
				failedClient := mock_idl.NewMockAgentClient(ctrl)
				failedClient.EXPECT().UpgradePrimaries(
					gomock.Any(),
					&idl.UpgradePrimariesRequest{
						SourceBinDir:               "/usr/local/greenplum-db/bin",
						TargetBinDir:               "/usr/local/greenplum-db-new/bin",
						TargetVersion:              semver.MustParse("6.0.0").String(),
						DataDirPairs:               pairs["sdw2"],
						CheckOnly:                  c.CheckOnly,
						UseLinkMode:                false,
						MasterBackupDir:            "",
						TablespacesMappingFilePath: "",
					},
				).Return(&idl.UpgradePrimariesReply{}, expected)

				agentConns := []*idl.Connection{
					{AgentClient: client1, Hostname: "sdw1"},
					{AgentClient: failedClient, Hostname: "sdw2"},
				}

				err := hub.UpgradePrimaries(hub.UpgradePrimaryArgs{
					CheckOnly:              c.CheckOnly,
					MasterBackupDir:        "",
					AgentConns:             agentConns,
					DataDirPairMap:         pairs,
					Source:                 source,
					IntermediateTarget:     target,
					UseLinkMode:            false,
					TablespacesMappingFile: "",
				})
				if err == nil {
					t.Fatal("expected error got nil")
				}

				// XXX it'd be nice if we didn't couple against a hardcoded string here,
				// but it's difficult to unwrap multiple errors with the new xerrors interface.
				if !strings.Contains(err.Error(), c.failedAction+" primary segment on host sdw2") ||
					!strings.Contains(err.Error(), expected.Error()) {
					t.Errorf("error %q did not contain expected contents '%q'", err.Error(), expected.Error())
				}
			})
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

		intermediateTarget := hub.MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
		})

		conf := &hub.Config{
			Source:             source,
			IntermediateTarget: intermediateTarget,
		}
		server := hub.New(conf, nil, "")

		_, err := server.GetDataDirPairs()
		if !errors.Is(err, hub.ErrInvalidCluster) {
			t.Errorf("returned error %#v got: %#v", err, hub.ErrInvalidCluster)
		}
	})

	t.Run("errors if source and target clusters have different content ids", func(t *testing.T) {
		source := hub.MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
			{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
			{ContentID: 1, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		})

		interemediateTarget := hub.MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
			{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
			{ContentID: 2, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		})

		conf := &hub.Config{
			Source:             source,
			IntermediateTarget: interemediateTarget,
		}
		server := hub.New(conf, nil, "")

		_, err := server.GetDataDirPairs()
		if !errors.Is(err, hub.ErrInvalidCluster) {
			t.Errorf("returned error %#v got: %#v", err, hub.ErrInvalidCluster)
		}
	})

	t.Run("errors if source and target cluster hostnames differ", func(t *testing.T) {
		source := hub.MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
			{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
			{ContentID: 1, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		})

		intermedaiteTarget := hub.MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
			{ContentID: 0, DbID: 2, Hostname: "localhost", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
			{ContentID: 1, DbID: 3, Hostname: "localhost", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		})

		conf := &hub.Config{
			Source:             source,
			IntermediateTarget: intermedaiteTarget,
		}
		server := hub.New(conf, nil, "")

		_, err := server.GetDataDirPairs()
		if !errors.Is(err, hub.ErrInvalidCluster) {
			t.Errorf("returned error %#v got: %#v", err, hub.ErrInvalidCluster)
		}
	})
}
