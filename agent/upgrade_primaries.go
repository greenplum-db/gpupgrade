package agent

import (
	"context"
	"os"
	"os/exec"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

// Allow exec.Command to be mocked out by exectest.NewCommand.
var execCommand = exec.Command

func (s *Server) UpgradePrimaries(ctx context.Context, in *idl.UpgradePrimariesRequest) (*idl.UpgradePrimariesReply, error) {
	gplog.Info("agent starting %s", upgradestatus.UPGRADE_PRIMARIES)

	err := UpgradePrimary(in.OldBinDir, in.NewBinDir, in.DataDirPairs, s.conf.StateDir, in.CheckOnly)
	return &idl.UpgradePrimariesReply{}, err
}

type Segment struct {
	*idl.DataDirPair

	WorkDir string // the pg_upgrade working directory, where logs are stored
}

func UpgradePrimary(sourceBinDir string, targetBinDir string, dataDirPairs []*idl.DataDirPair, stateDir string, checkOnly bool) error {
	segments := make([]Segment, 0, len(dataDirPairs))

	for _, dataPair := range dataDirPairs {
		workdir := utils.SegmentPGUpgradeDirectory(stateDir, int(dataPair.Content))
		err := utils.System.MkdirAll(workdir, 0700)
		if err != nil {
			return xerrors.Errorf("upgrading primaries: %w", err)
		}

		segments = append(segments, Segment{
			DataDirPair: dataPair,
			WorkDir:     workdir,
		})
	}

	err := UpgradeSegments(sourceBinDir, targetBinDir, segments, checkOnly)
	if err != nil {
		return errors.Wrap(err, "failed to upgrade segments")
	}

	return nil
}

func UpgradeSegments(sourceBinDir string, targetBinDir string, segments []Segment, checkOnly bool) (err error) {
	host, err := os.Hostname()
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	agentErrs := make(chan error, len(segments))

	for _, segment := range segments {
		dbid := int(segment.DBID)
		segmentPair := upgrade.SegmentPair{
			Source: &upgrade.Segment{sourceBinDir, segment.OldDataDir, dbid, int(segment.OldPort)},
			Target: &upgrade.Segment{targetBinDir, segment.NewDataDir, dbid, int(segment.NewPort)},
		}

		options := []upgrade.Option{
			upgrade.WithExecCommand(execCommand),
			upgrade.WithWorkDir(segment.WorkDir),
			upgrade.WithSegmentMode(),
		}
		if checkOnly {
			options = append(options, upgrade.WithCheckOnly())
		}

		content := segment.Content
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := upgrade.Run(segmentPair, options...)
			if err != nil {
				failedAction := "upgrade"
				if checkOnly {
					failedAction = "check"
				}
				agentErrs <- errors.Wrapf(err, "failed to %s primary on host %s with content %d", failedAction, host, content)
			}
		}()
	}

	wg.Wait()
	close(agentErrs)

	for agentErr := range agentErrs {
		err = multierror.Append(err, agentErr)
	}

	return err
}
