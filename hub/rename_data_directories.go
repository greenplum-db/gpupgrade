// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

var ArchiveSource = upgrade.ArchiveSource

type RenameMap = map[string][]*idl.RenameDirectories

func (s *Server) UpdateDataDirectories() error {
	return UpdateDataDirectories(s.Config, s.agentConns)
}

func UpdateDataDirectories(conf *Config, agentConns []*idl.Connection) error {
	source := conf.Source.MasterDataDir()
	target := conf.IntermediateTarget.Master.DataDir
	if err := ArchiveSource(source, target, true); err != nil {
		return xerrors.Errorf("renaming master data directories: %w", err)
	}

	// in link mode, remove the source mirror and standby data directories; otherwise we create a second copy
	//  of them for the target cluster. That might take too much disk space.
	if conf.UseLinkMode {
		if err := DeleteMirrorAndStandbyDataDirectories(agentConns, conf.Source); err != nil {
			return xerrors.Errorf("removing source cluster standby and mirror segment data directories: %w", err)
		}

		if err := DeleteSourceTablespacesOnMirrorsAndStandby(agentConns, conf.Source, conf.Tablespaces); err != nil {
			return xerrors.Errorf("removing source cluster standby and mirror tablespace data directories: %w", err)
		}
	}

	renameMap := getRenameMap(conf.Source, conf.IntermediateTarget, conf.UseLinkMode)
	if err := RenameSegmentDataDirs(agentConns, renameMap); err != nil {
		return xerrors.Errorf("renaming segment data directories: %w", err)
	}

	return nil
}

// getRenameMap() returns a map of host to cluster data directories to be renamed.
// This includes renaming source to archive, and target to source. In link mode
// the mirrors have been deleted to save disk space, so exclude them from the map.
// Since the upgraded mirrors will be added later to the correct directory there
// is no need to rename target to source, so only archive the source directory.
func getRenameMap(source *greenplum.Cluster, target InitializeConfig, onlyRenamePrimaries bool) RenameMap {
	m := make(RenameMap)

	for _, seg := range target.Primaries {
		m[seg.Hostname] = append(m[seg.Hostname], &idl.RenameDirectories{
			Source:       source.Primaries[seg.ContentID].DataDir,
			Target:       seg.DataDir,
			RenameTarget: true,
		})
	}

	// In link mode the mirrors have been deleted to save disk space, so exclude
	// them from the map.
	if onlyRenamePrimaries {
		return m
	}

	targetMirrors := append(target.Mirrors, target.Standby)
	for _, seg := range targetMirrors {
		m[seg.Hostname] = append(m[seg.Hostname], &idl.RenameDirectories{
			Source:       source.Mirrors[seg.ContentID].DataDir,
			Target:       seg.DataDir,
			RenameTarget: false,
		})
	}

	return m
}

// e.g. for source /data/dbfast1/demoDataDir0 becomes /data/dbfast1/demoDataDir0_old
// e.g. for target /data/dbfast1/demoDataDir0_123ABC becomes /data/dbfast1/demoDataDir0
func RenameSegmentDataDirs(agentConns []*idl.Connection, renames RenameMap) error {
	request := func(conn *idl.Connection) error {
		if len(renames[conn.Hostname]) == 0 {
			return nil
		}

		req := &idl.RenameDirectoriesRequest{Dirs: renames[conn.Hostname]}
		_, err := conn.AgentClient.RenameDirectories(context.Background(), req)
		return err
	}

	return ExecuteRPC(agentConns, request)
}
