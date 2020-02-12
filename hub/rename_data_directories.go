package hub

import (
	"context"
	"path/filepath"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

const OldSuffix = "_old"
const UpgradeSuffix = "_upgrade"

func (s *Server) RenameDataDirectories() error {
	if err := RenameMasterDataDir(s.Source.MasterDataDir(), true); err != nil {
		return xerrors.Errorf("renaming old cluster master data directory: %w", err)
	}

	if err := RenameSegmentDataDirs(s.agentConns, s.Source, OldSuffix, false); err != nil {
		return xerrors.Errorf("renaming old cluster segment data directories: %w", err)
	}

	if err := RenameMasterDataDir(s.Target.MasterDataDir(), false); err != nil {
		return xerrors.Errorf("renaming new cluster master data directory: %w", err)
	}

	if err := RenameSegmentDataDirs(s.agentConns, s.Target, UpgradeSuffix, true); err != nil {
		return xerrors.Errorf("renaming new cluster segment data directories: %w", err)
	}

	return nil
}

// e.g. for source /data/qddir/demoDataDir-1 becomes /data/qddir_old/demoDataDir-1
// e.g. for target /data/qddir_upgrade/demoDataDir-1 becomes /data/qddir/demoDataDir-1
func RenameMasterDataDir(masterDataDir string, isSource bool) error {
	destination := "new"
	src := filepath.Dir(masterDataDir) + UpgradeSuffix
	dst := filepath.Dir(masterDataDir)
	if isSource {
		destination = "old"
		src = filepath.Dir(masterDataDir)
		dst = filepath.Dir(masterDataDir) + OldSuffix
	}
	if err := utils.System.Rename(src, dst); err != nil {
		return xerrors.Errorf("renaming %s cluster master data directory from: '%s' to: '%s': %w", destination, src, dst, err)
	}
	return nil
}

// TODO: Update RenameSegmentDataDirs to include renaming the standby by
//  changing SegmentsOn to return the standby. Careful since this is used by
//  other callers. AgentConns() also needs to be updated to create an
//  agentConn on the standby host.
// e.g. for source /data/dbfast1/demoDataDir0 becomes datadirs/dbfast1_old/demoDataDir0
// e.g. for target /data/dbfast1_upgrade/demoDataDir0 becomes datadirs/dbfast1/demoDataDir0
func RenameSegmentDataDirs(agentConns []*Connection,
	cluster *utils.Cluster,
	suffix string,
	addSuffixToSrc bool) error {

	wg := sync.WaitGroup{}
	errs := make(chan error, len(agentConns))

	for _, conn := range agentConns {
		wg.Add(1)

		go func(c *Connection) {
			defer wg.Done()

			segments, err := cluster.SegmentsOn(c.Hostname)
			if err != nil {
				errs <- err
				return
			}

			// When there are multiple segments under a parent data directory
			// only call rename once on the parent directory.
			// For example, /data/primary/gpseg1 and /data/primary/gpseg2
			// only call rename once for /data/primary.
			parentDirs := make(map[string]bool)
			for _, seg := range segments {
				parentDirs[filepath.Dir(seg.DataDir)] = true
			}

			req := new(idl.RenameDirectoriesRequest)
			for dir := range parentDirs {
				dst := dir
				src := dir
				if addSuffixToSrc {
					src = dir + suffix
				} else {
					dst = dir + suffix
				}

				req.Pairs = append(req.Pairs, &idl.RenamePair{Src: src, Dst: dst})
			}

			_, err = c.AgentClient.RenameDirectories(context.Background(), req)
			if err != nil {
				gplog.Error("renaming segment data directories on host %s: %s", c.Hostname, err.Error())
				errs <- err
			}
		}(conn)
	}

	wg.Wait()
	close(errs)

	var mErr *multierror.Error
	for err := range errs {
		mErr = multierror.Append(mErr, err)
	}

	return mErr.ErrorOrNil()
}
