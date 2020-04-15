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
	if err := upgrade.RenameDataDirectory(source, source+upgrade.OldSuffix, target); err != nil {
		return xerrors.Errorf("renaming master data directories: %w", err)
	}

	// in --link mode, remove the source mirror and standby data directories; otherwise we create a second copy
	//  of them for the target cluster. That might take too much disk space.
	if conf.UseLinkMode {
		if err := DeleteMirrorAndStandbyDirectories(agentConns, conf.Source); err != nil {
			return xerrors.Errorf("removing source cluster standby and mirror segment data directories: %w", err)
		}
	}

	renameMap := getSourceRenameMap(conf.Source, conf.UseLinkMode)
	if err := RenameSegmentDataDirs(agentConns, renameMap); err != nil {
		return xerrors.Errorf("renaming source cluster segment data directories: %w", err)
	}

	renameMap = getTargetRenameMap(conf.TargetInitializeConfig, conf.Source)
	if err := RenameSegmentDataDirs(agentConns, renameMap); err != nil {
		return xerrors.Errorf("renaming target cluster segment data directories: %w", err)
	}

	return nil
}

func getSourceRenameMap(source *greenplum.Cluster, primariesOnly bool) RenameMap {
	m := make(RenameMap)

	for _, content := range source.ContentIDs {
		seg := source.Primaries[content]
		if !seg.IsMaster() {
			m[seg.Hostname] = append(m[seg.Hostname], &idl.RenamePair{
				Src: seg.DataDir,
				Dst: seg.DataDir + upgrade.OldSuffix,
			})
		}

		seg, ok := source.Mirrors[content]
		if !primariesOnly && ok {
			m[seg.Hostname] = append(m[seg.Hostname], &idl.RenamePair{
				Src: seg.DataDir,
				Dst: seg.DataDir + upgrade.OldSuffix,
			})
		}
	}

	return m
}

// getTargetRenameMap returns a rename map in which all primary target data directories
// are renamed to their corresponding source directories.
func getTargetRenameMap(target InitializeConfig, source *greenplum.Cluster) RenameMap {
	m := make(RenameMap)

	// Do not include mirrors and stand by when moving _upgrade directories,
	// since they don't exist yet.  Master is renamed in a separate function.
	for _, targetSeg := range target.Primaries {
		content := targetSeg.ContentID
		sourceSeg := source.Primaries[content]

		host := targetSeg.Hostname
		m[host] = append(m[host], &idl.RenamePair{
			Src: targetSeg.DataDir,
			Dst: sourceSeg.DataDir,
		})
	}

	return m
}

// e.g. for source /data/dbfast1/demoDataDir0 becomes datadirs/dbfast1/demoDataDir0_old
// e.g. for target /data/dbfast1/demoDataDir0_123ABC becomes datadirs/dbfast1/demoDataDir0
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
