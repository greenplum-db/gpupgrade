package services

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/blang/semver"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

func (s *AgentServer) UpgradeConvertPrimarySegments(ctx context.Context, in *pb.UpgradeConvertPrimarySegmentsRequest) (*pb.UpgradeConvertPrimarySegmentsReply, error) {
	gplog.Info("got a request to convert primary from the hub")

	err := s.UpgradeSegments(in)

	if err != nil {
		return &pb.UpgradeConvertPrimarySegmentsReply{}, err
	}

	return &pb.UpgradeConvertPrimarySegmentsReply{}, nil
}

func (s *AgentServer) UpgradeSegments(in *pb.UpgradeConvertPrimarySegmentsRequest) error {
	filename := "pg_upgrade_dump_*_oids.sql"
	shareOIDfilePath := filepath.Join(utils.PGUpgradeDirectory(s.conf.StateDir), filename)
	oidFiles, err := utils.System.FilePathGlob(shareOIDfilePath)
	if err != nil {
		gplog.Error("ls OID files failed. Err: %v", err)
		return err
	}
	//len(nil) = 0
	if len(oidFiles) == 0 {
		gplog.Error("Share OID files do not exist. Pattern was: %s", shareOIDfilePath)
		return errors.New("No OID files found")
	}

	targetVersion, err := semver.Parse(in.NewVersion)
	if err != nil {
		gplog.Error("failed to parse new cluster version: %s", err)
		return errors.Wrap(err, "failed to parse new cluster version")
	}

	// TODO: consolidate this logic with Hub.ConvertMaster().

	// pg_upgrade changed its API in GPDB 6.0.
	var modeStr string
	if targetVersion.LT(semver.MustParse("6.0.0")) {
		modeStr = "--progress"
	} else {
		modeStr = "--mode=segment"
	}

	for _, segment := range in.DataDirPairs {
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

		convertPrimaryCmd := fmt.Sprintf("cd %s && nohup %s --old-bindir=%s --old-datadir=%s --new-bindir=%s --new-datadir=%s --old-port=%d --new-port=%d %s",
			pathToSegment, in.NewBinDir+"/pg_upgrade", in.OldBinDir, segment.OldDataDir, in.NewBinDir, segment.NewDataDir, segment.OldPort, segment.NewPort, modeStr)

		err = utils.System.RunCommandAsync(convertPrimaryCmd, filepath.Join(pathToSegment, "pg_upgrade_segment.log"))
		if err != nil {
			gplog.Error("An error occurred: %v", err)
			return err
		}
	}

	return nil
}
