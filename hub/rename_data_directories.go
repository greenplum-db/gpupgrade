package hub

import (
	"context"
	"path/filepath"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

const OldSuffix = "_old"
const UpgradeSuffix = "_upgrade"

func (s *Server) RenameDataDirectories() error {
	if err := RenameMasterDataDir(s.Source.MasterDataDir(), true); err != nil {
		return xerrors.Errorf("renaming source cluster master data directory: %w", err)
	}

	// Include mirror and standby directories in the _old archiving.
	// TODO: in --link mode, shouldn't we be _removing_ the mirror and standby
	// directories?
	if err := RenameSegmentDataDirs(s.agentConns, s.Source, "", OldSuffix, false); err != nil {
		return xerrors.Errorf("renaming source cluster segment data directories: %w", err)
	}

	if err := RenameMasterDataDir(s.Target.MasterDataDir(), false); err != nil {
		return xerrors.Errorf("renaming target cluster master data directory: %w", err)
	}

	// Do not include mirrors and standby when moving _upgrade directories,
	// since they don't exist yet.
	if err := RenameSegmentDataDirs(s.agentConns, s.Target, UpgradeSuffix, "", true); err != nil {
		return xerrors.Errorf("renaming target cluster segment data directories: %w", err)
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

// e.g. for source /data/dbfast1/demoDataDir0 becomes datadirs/dbfast1_old/demoDataDir0
// e.g. for target /data/dbfast1_upgrade/demoDataDir0 becomes datadirs/dbfast1/demoDataDir0
func RenameSegmentDataDirs(agentConns []*Connection,
	cluster *greenplum.Cluster,
	oldSuffix, newSuffix string,
	primariesOnly bool) error {

	wg := sync.WaitGroup{}
	errs := make(chan error, len(agentConns))

	for _, conn := range agentConns {
		conn := conn

		selector := func(seg *greenplum.SegConfig) bool {
			if !seg.IsOnHost(conn.Hostname) || seg.IsMaster() {
				return false
			}

			if primariesOnly {
				return seg.IsPrimary()
			}

			// Otherwise include mirrors and standby. (Master's excluded above.)
			return true
		}

		segments := cluster.SelectSegments(selector)
		if len(segments) == 0 {
			// we can have mirror-only and standby-only hosts, which we don't
			// care about here (they are added later)
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			// When there are multiple segments under a parent data directory
			// only call rename once on the parent directory.
			// For example, /data/primary/gpseg1 and /data/primary/gpseg2
			// only call rename once for /data/primary.
			// NOTE: we keep the iteration stable for testing purposes; hence
			// the combined map+slice approach.
			alreadyDone := make(map[string]bool)
			var parentDirs []string

			for _, seg := range segments {
				// For most segments, we want to rename the parent.
				dir := filepath.Dir(seg.DataDir)
				if seg.IsStandby() {
					// Standby follows different naming rules; we rename its
					// data directory directly.
					dir = seg.DataDir
				}

				if alreadyDone[dir] {
					continue
				}

				parentDirs = append(parentDirs, dir)
				alreadyDone[dir] = true
			}

			req := new(idl.RenameDirectoriesRequest)
			for _, dir := range parentDirs {
				src := dir + oldSuffix
				dst := dir + newSuffix

				req.Pairs = append(req.Pairs, &idl.RenamePair{Src: src, Dst: dst})
			}

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
