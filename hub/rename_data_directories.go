package hub

import (
	"context"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

type RenameMap = map[string][]*idl.RenamePair

func (s *Server) UpdateDataDirectories() error {
	return UpdateDataDirectories(s.Config, s.agentConns)
}

func UpdateDataDirectories(conf *Config, agentConns []*Connection) error {
	source := conf.Source.MasterDataDir()
	target := conf.TargetInitializeConfig.Master.DataDir
	if err := upgrade.RenameDataDirectory(source, source+upgrade.OldSuffix, target, true); err != nil {
		return xerrors.Errorf("renaming master data directories: %w", err)
	}

	// in --link mode, remove the source mirror and standby data directories; otherwise we create a second copy
	//  of them for the target cluster. That might take too much disk space.
	if conf.UseLinkMode {
		if err := DeleteMirrorAndStandbyDirectories(agentConns, conf.Source); err != nil {
			return xerrors.Errorf("removing source cluster standby and mirror segment data directories: %w", err)
		}
	}

	renameMap := getRenameMap(conf.Source, conf.TargetInitializeConfig, conf.UseLinkMode)
	if err := RenameSegmentDataDirs(agentConns, renameMap); err != nil {
		return xerrors.Errorf("renaming segment data directories: %w", err)
	}

	return nil
}

// getRenameMap() returns a map of host to cluster data directories to be renamed.
// This includes renaming source to archive, and target to source. In link mode
// the mirrors have been deleted to save disk space, so exclude them from the map.
// Since the upgraded mirrors will be added later to the correct directory there
// is no need to rename target to source.
func getRenameMap(source *greenplum.Cluster, target InitializeConfig, sourcePrimariesOnly bool) RenameMap {
	m := make(RenameMap)
	targetMap := make(map[int]string)

	// Do not include mirrors and standby when moving target directories,
	// since they don't exist yet.  Master is renamed in a separate function.
	for _, targetSeg := range target.Primaries {
		targetMap[targetSeg.ContentID] = targetSeg.DataDir
	}

	for _, content := range source.ContentIDs {
		seg := source.Primaries[content]
		if !seg.IsMaster() {
			m[seg.Hostname] = append(m[seg.Hostname], &idl.RenamePair{
				Src:          seg.DataDir,
				Archive:      seg.DataDir + upgrade.OldSuffix,
				Dst:          targetMap[content],
				RenameTarget: true,
			})
		}

		seg, ok := source.Mirrors[content]
		if !sourcePrimariesOnly && ok {
			m[seg.Hostname] = append(m[seg.Hostname], &idl.RenamePair{
				Src:     seg.DataDir,
				Archive: seg.DataDir + upgrade.OldSuffix,
			})
		}
	}

	return m
}

// e.g. for source /data/dbfast1/demoDataDir0 becomes /data/dbfast1/demoDataDir0_old
// e.g. for target /data/dbfast1/demoDataDir0_123ABC becomes /data/dbfast1/demoDataDir0
func RenameSegmentDataDirs(agentConns []*Connection, renames RenameMap) error {
	wg := sync.WaitGroup{}
	errs := make(chan error, len(agentConns))

	for _, conn := range agentConns {
		conn := conn

		if len(renames[conn.Hostname]) == 0 {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			req := &idl.RenameDirectoriesRequest{Pairs: renames[conn.Hostname]}
			_, err := conn.AgentClient.RenameDirectories(context.Background(), req)
			if err != nil {
				gplog.Error("renaming segment data directories on host %s: %s", conn.Hostname, err.Error())
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	var mErr *multierror.Error
	for err := range errs {
		mErr = multierror.Append(mErr, err)
	}

	return mErr.ErrorOrNil()
}
