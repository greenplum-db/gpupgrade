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

func RenameDataDirectories(agentConns []*idl.Connection, source *greenplum.Cluster, intermediateTarget *greenplum.Cluster, linkMode bool) error {
	src := source.MasterDataDir()
	dst := intermediateTarget.MasterDataDir()
	if err := ArchiveSource(src, dst, true); err != nil {
		return xerrors.Errorf("renaming master data directories: %w", err)
	}

	renameMap := getRenameMap(source, intermediateTarget, linkMode)
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
func getRenameMap(source *greenplum.Cluster, intermediateTarget *greenplum.Cluster, onlyRenamePrimaries bool) RenameMap {
	m := make(RenameMap)

	for _, seg := range source.Primaries {
		if seg.IsMaster() {
			continue
		}

		m[seg.Hostname] = append(m[seg.Hostname], &idl.RenameDirectories{
			Source:       seg.DataDir,
			Target:       intermediateTarget.Primaries[seg.ContentID].DataDir,
			RenameTarget: true,
		})
	}

	// In link mode the mirrors have been deleted to save disk space, so exclude
	// them from the map.
	if onlyRenamePrimaries {
		return m
	}

	for _, seg := range source.Mirrors {
		m[seg.Hostname] = append(m[seg.Hostname], &idl.RenameDirectories{
			Source:       seg.DataDir,
			Target:       intermediateTarget.Mirrors[seg.ContentID].DataDir,
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
