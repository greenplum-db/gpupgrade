package services

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (s *AgentServer) UpgradeConvertPrimarySegments(ctx context.Context, in *idl.UpgradeConvertPrimarySegmentsRequest) (*idl.UpgradeConvertPrimarySegmentsReply, error) {
	gplog.Info("got a request to convert primary from the hub")

	err := s.UpgradeSegments(in.OldBinDir, in.NewBinDir, in.DataDirPairs)

	if err != nil {
		return &idl.UpgradeConvertPrimarySegmentsReply{}, err
	}

	return &idl.UpgradeConvertPrimarySegmentsReply{}, nil
}

func (s *AgentServer) UpgradeSegments(oldBinDir string, newBinDir string, dataDirPairs []*idl.DataDirPair) error {
	// TODO: consolidate this logic with Hub.ConvertMaster().

	for _, segment := range dataDirPairs {
		pathToSegment := utils.SegmentPGUpgradeDirectory(s.conf.StateDir, int(segment.Content))
		err := utils.System.MkdirAll(pathToSegment, 0700)
		if err != nil {
			gplog.Error("Could not create segment directory. Err: %v", err)
			return err
		}

		cmd := fmt.Sprintf("source %s; cd %s; unset PGHOST; unset PGPORT; "+
			"%s --old-bindir=%s --old-datadir=%s --old-port=%d "+
			"--new-bindir=%s --new-datadir=%s --new-port=%d --mode=segment --progress",
			filepath.Join(newBinDir, "..", "greenplum_path.sh"),
			pathToSegment,
			filepath.Join(newBinDir, "pg_upgrade"),
			oldBinDir,
			segment.OldDataDir,
			segment.OldPort,
			newBinDir,
			segment.NewDataDir,
			segment.NewPort)

		// TODO: do this synchronously.
		err = utils.System.RunCommandAsync(cmd, filepath.Join(pathToSegment, "pg_upgrade_segment.log"))
		if err != nil {
			gplog.Error("An error occurred: %v", err)
			return err
		}
	}

	return nil
}
