package hub

import (
	"context"
	"os"
	"sync"
	"syscall"

	"github.com/greenplum-db/gpupgrade/upgrade"

	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

type RenameMap = map[string][]*idl.RenameDataDirs

func (s *Server) UpdateDataDirectories() error {
	return UpdateDataDirectories(s.Config, s.agentConns)
}

// UpdateDataDirectories renames the data directories of the source cluster to archive directories
//   and the data directories of the target cluster to the original source cluster locations.  That is:
//        DIR/sourceDataDir -> DIR/sourceDataDir.ABC123.old
//        DIR/targetDataDir -> DIR/sourceDataDir
//  for each data directory.
// Note that the target cluster does not have a standby or mirrors at this point.
func UpdateDataDirectories(conf *Config, agentConns []*Connection) error {
	if err := RenameDataDirs(conf.Source.MasterDataDir(), conf.TargetInitializeConfig.Master.DataDir, conf.UpgradeID); err != nil {
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
	if err := RenameSegmentDataDirs(agentConns, renameMap, conf.UpgradeID); err != nil {
		return xerrors.Errorf("renaming source cluster segment data directories: %w", err)
	}

	return nil
}

// getRenameMap() splices together the source and target clusters by combining the corresponding segment from
//   each cluster.  It does so per-host in order to enable a single hub-agent command for renaming dataDirs.
// TODO: Do we want to sanity-check that the source and target clusters "match"?  At this point in finalize,
//   this should be a reasonable assumption.
func getRenameMap(source *greenplum.Cluster, target InitializeConfig, sourcePrimariesOnly bool) RenameMap {
	m := make(RenameMap)
	tMap := make(map[int]string)

	// Do not include mirrors and standby when moving target directories,
	// since they don't exist yet.  Master is renamed in a separate function.
	for _, targetSeg := range target.Primaries {
		tMap[targetSeg.ContentID] = targetSeg.DataDir
	}

	for _, content := range source.ContentIDs {
		seg := source.Primaries[content]
		if !seg.IsMaster() {
			m[seg.Hostname] = append(m[seg.Hostname], &idl.RenameDataDirs{
				Source: seg.DataDir,
				Target: tMap[content],
			})
		}

		seg, ok := source.Mirrors[content]
		if !sourcePrimariesOnly && ok {
			m[seg.Hostname] = append(m[seg.Hostname], &idl.RenameDataDirs{
				Source: seg.DataDir,
			})
		}
	}

	return m
}

// IsRenameErrorIdempotent interprets an error returned from os.Rename().  If that error is acceptable, it returns true.
// The error code options are taken from the POSIX rename(2) manpage:
//   https://pubs.opengroup.org/onlinepubs/009695399/functions/rename.html
// These are consistent with the Mac OSX manpage and Linux manpage for rename(2.
// Note that if rename(2) could return both ENOENT and (EEXIST,ENOTEMPTY),  the standard does
// not specify which is returned.
func IsRenameErrorIdempotent(err error) bool {
	switch x := err.(type) {
	case *os.LinkError:
		if xerrors.Is(x.Err, syscall.ENOENT) {
			gplog.Info("rename already run: old dir not present: %v (%v)", x, x.Err)
			return true
		} else if xerrors.Is(x.Err, syscall.EEXIST) || xerrors.Is(x.Err, syscall.ENOTEMPTY) {
			gplog.Info("rename already run: new dir present: %v (%v)", x, x.Err)
			return true
		}
	}

	return false
}

func AlreadyRenamed(source, target, archive string, noTargetMirror bool) bool {
	if noTargetMirror {
		return !utils.DoesPathExist(source) && utils.DoesPathExist(archive)
	}
	return utils.DoesPathExist(source) && utils.DoesPathExist(archive) &&
		!utils.DoesPathExist(target)

}

// RenameDataDirs uses os.Rename() to
//   1). archive the source master dataDir
//   2). if present, move the target dataDir where the source dataDir used to be.
// Since os.Rename() is atomic, either neither, just 1), or both 1) and 2) occur.
// Both source and target are renamed in a single function since they are coupled:
//   the idempotence checks of the source rename depends on whether the target
//   has been renamed successfully.
func RenameDataDirs(source, target string, upgradeID upgrade.ID) error {
	noTargetMirror := target == ""

	archive := upgrade.ArchiveDirectoryForSource(source, upgradeID)

	if AlreadyRenamed(source, target, archive, noTargetMirror) {
		return nil
	}

	if err := utils.System.Rename(source, archive); err != nil {
		if !IsRenameErrorIdempotent(err) {
			return xerrors.Errorf("renaming source: %w", err)
		}
	}

	// target mirror/standby don't exist yet
	if noTargetMirror {
		return nil
	}

	if err := utils.System.Rename(target, source); err != nil {
		if !IsRenameErrorIdempotent(err) {
			return xerrors.Errorf("renaming target: %w", err)
		}
	}

	return nil
}

// RenameSegmentDataDirs() delegates renaming of the segment dataDirs to the agents.
func RenameSegmentDataDirs(agentConns []*Connection, renames RenameMap, upgradeID upgrade.ID) error {

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

			req := &idl.RenameDataDirectoriesRequest{DataDirs: renames[conn.Hostname], UpgradeID: uint64(upgradeID)}
			_, err := conn.AgentClient.RenameDataDirectories(context.Background(), req)
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
