package services

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/blang/semver"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

func (s *AgentServer) UpgradeConvertPrimarySegments(ctx context.Context, in *idl.UpgradeConvertPrimarySegmentsRequest) (*idl.UpgradeConvertPrimarySegmentsReply, error) {
	gplog.Info("got a request to convert primary from the hub")

	err := s.UpgradeSegments(in.OldBinDir, in.NewBinDir, in.NewVersion, in.DataDirPairs)

	if err != nil {
		return &idl.UpgradeConvertPrimarySegmentsReply{}, err
	}

	return &idl.UpgradeConvertPrimarySegmentsReply{}, nil
}

func (s *AgentServer) UpgradeSegments(oldBinDir, newBinDir, newVersion string, dataDirPairs []*idl.DataDirPair) error {
	// OID files to copy from the master. Empty/unused in GPDB 6 and higher.
	var oidFiles []string

	targetVersion, err := semver.Parse(newVersion)
	if err != nil {
		gplog.Error("failed to parse new cluster version: %s", err)
		return errors.Wrap(err, "failed to parse new cluster version")
	}

	if targetVersion.LT(semver.MustParse("6.0.0")) {
		var err error // to prevent shadowing of oidFiles

		// For GPDB 5.x and below, the pg_upgrade OID files have already been
		// transferred from the master to the agent state directory.
		filename := "pg_upgrade_dump_*_oids.sql"
		shareOIDfilePath := filepath.Join(utils.PGUpgradeDirectory(s.conf.StateDir), filename)
		oidFiles, err = utils.System.FilePathGlob(shareOIDfilePath)
		if err != nil {
			gplog.Error("ls OID files failed. Err: %v", err)
			return err
		}

		if len(oidFiles) == 0 {
			gplog.Error("Share OID files do not exist. Pattern was: %s", shareOIDfilePath)
			return errors.New("No OID files found")
		}
	}

	// TODO: consolidate this logic with Hub.ConvertMaster().

	// pg_upgrade changed its API in GPDB 6.0.
	var modeStr string
	if targetVersion.LT(semver.MustParse("6.0.0")) {
		modeStr = "--progress"
	} else {
		modeStr = "--mode=segment --progress"
	}

	for _, segment := range dataDirPairs {
		pathToSegment := utils.SegmentPGUpgradeDirectory(s.conf.StateDir, int(segment.Content))
		err := utils.System.MkdirAll(pathToSegment, 0700)
		if err != nil {
			gplog.Error("Could not create segment directory. Err: %v", err)
			return err
		}

		for _, oidFile := range oidFiles {
			out, err := s.executor.ExecuteLocalCommand(fmt.Sprintf("cp %s %s", oidFile, pathToSegment))
			if err != nil {
				gplog.Error("Failed to copy OID files for segment %d. Output: %s", segment.Content, string(out))
				return err
			}
		}

		cmd := fmt.Sprintf("source %s; cd %s; unset PGHOST; unset PGPORT; "+
			"%s --old-bindir=%s --old-datadir=%s --old-port=%d "+
			"--new-bindir=%s --new-datadir=%s --new-port=%d %s",
			filepath.Join(newBinDir, "..", "greenplum_path.sh"),
			pathToSegment,
			filepath.Join(newBinDir, "pg_upgrade"),
			oldBinDir,
			segment.OldDataDir,
			segment.OldPort,
			newBinDir,
			segment.NewDataDir,
			segment.NewPort,
			modeStr)

		// TODO: do this synchronously.
		err = utils.System.RunCommandAsync(cmd, filepath.Join(pathToSegment, "pg_upgrade_segment.log"))
		if err != nil {
			gplog.Error("An error occurred: %v", err)
			return err
		}
	}

	return nil
}
