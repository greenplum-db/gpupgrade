package hub

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

const OldSuffix = "_old"

func (s *Server) UpdateDataDirectories() error {
	if err := RenameMasterDataDir(s.Source.MasterDataDir(), "", true); err != nil {
		return xerrors.Errorf("renaming source cluster master data directory: %w", err)
	}

	// in --link mode, remove the mirror and standby data directories
	if s.Config.UseLinkMode {
		if err := DeleteMirrorAndStandbyDirectories(s.agentConns, s.Source); err != nil {
			return xerrors.Errorf("removing source cluster standby and mirror segment data directories: %w", err)
		}
	}

	if err := RenameSegmentDataDirs(s.agentConns, s.Source, nil, OldSuffix, s.Config.UseLinkMode); err != nil {
		return xerrors.Errorf("renaming source cluster segment data directories: %w", err)
	}

	if err := RenameMasterDataDir(s.Target.MasterDataDir(), s.TargetInitializeConfig.Master.DataDir, false); err != nil {
		return xerrors.Errorf("renaming target cluster master data directory: %w", err)
	}

	// Do not include mirrors and standby when moving _upgrade directories,
	// since they don't exist yet.
	var targetSegs []greenplum.SegConfig
	targetSegs = append(targetSegs, s.TargetInitializeConfig.Master)
	targetSegs = append(targetSegs, s.TargetInitializeConfig.Primaries...)
	origTarget, err := greenplum.NewCluster(targetSegs)
	if err != nil {
		return xerrors.Errorf("forming old target cluster failed: %w, err")
	}
	if err = RenameSegmentDataDirs(s.agentConns, origTarget, s.Target, "", true); err != nil {
		return xerrors.Errorf("renaming target cluster segment data directories: %w", err)
	}

	return nil
}

// e.g. for source /data/qddir/demoDataDir-1 becomes /data/qddir/demoDataDir-1_old
// e.g. for target /data/qddir/demoDataDir-1_123GNHFD3 becomes /data/qddir/demoDataDir-1
// TODO: rework this interface
func RenameMasterDataDir(masterDataDir string, targetMasterDataDir string, isSource bool) error {
	dstTag := "target"
	src := targetMasterDataDir
	dst := masterDataDir
	if isSource {
		dstTag = "source"
		src = masterDataDir
		dst = masterDataDir + OldSuffix
	}
	if err := utils.System.Rename(src, dst); err != nil {
		return xerrors.Errorf("renaming %s cluster master data directory from: '%s' to: '%s': %w", dstTag, src, dst, err)
	}
	return nil
}

// e.g. for source /data/dbfast1/demoDataDir0 becomes datadirs/dbfast1/demoDataDir0_old
// e.g. for target /data/dbfast1/demoDataDir0_123ABC becomes datadirs/dbfast1/demoDataDir0
func RenameSegmentDataDirs(agentConns []*Connection, srcCluster, dstCluster *greenplum.Cluster, newSuffix string, primariesOnly bool) error {

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

		segments := srcCluster.SelectSegments(selector)
		if len(segments) == 0 {
			// we can have mirror-only and standby-only hosts, which we don't
			// care about here (they are added later)
			continue
		}
		var dstSegments []greenplum.SegConfig
		if dstCluster != nil {
			dstSegments = dstCluster.SelectSegments(selector)
			if len(segments) != len(dstSegments) {
				err := errors.New(fmt.Sprintf("src and dst cluster mismatch: src: %v dst: %v", segments, dstSegments))
				gplog.Error(fmt.Sprintf("%v", err))
				errs <- err
				continue
			}
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			req := new(idl.RenameDirectoriesRequest)
			for _, seg := range segments {
				src := seg.DataDir
				dst := seg.DataDir + newSuffix
				if dstCluster != nil {
					found := false
					for _, dseg := range dstSegments {
						if dseg.ContentID == seg.ContentID && dseg.DbID == seg.DbID {
							dst = dseg.DataDir
							found = true
							break
						}
					}
					if !found {
						err := errors.New(fmt.Sprintf("no matching dstSeg for seg %v", seg))
						gplog.Error(fmt.Sprintf("%v", err))
						errs <- err
						return
					}
				}

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
