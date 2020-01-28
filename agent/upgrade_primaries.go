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

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

// Allow exec.Command to be mocked out by exectest.NewCommand.
var execCommand = exec.Command

func (s *Server) UpgradePrimaries(ctx context.Context, request *idl.UpgradePrimariesRequest) (*idl.UpgradePrimariesReply, error) {
	gplog.Info("agent starting %s", idl.Substep_UPGRADE_PRIMARIES)

	err := UpgradePrimary(s.conf.StateDir, request)
	return &idl.UpgradePrimariesReply{}, err
}

type Segment struct {
	*idl.DataDirPair

	WorkDir string // the pg_upgrade working directory, where logs are stored
}

func UpgradePrimary(stateDir string, request *idl.UpgradePrimariesRequest) error {
	segments := make([]Segment, 0, len(request.DataDirPairs))

	for _, dataPair := range request.DataDirPairs {
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

	err := UpgradeSegments(segments, request)
	if err != nil {
		return errors.Wrap(err, "failed to upgrade segments")
	}

	return nil
}

func UpgradeSegments(segments []Segment, request *idl.UpgradePrimariesRequest) (err error) {
	host, err := os.Hostname()
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	agentErrs := make(chan error, len(segments))

	for _, segment := range segments {
		dbid := int(segment.DBID)
		segmentPair := upgrade.SegmentPair{
			Source: &upgrade.Segment{request.SourceBinDir, segment.SourceDataDir, dbid, int(segment.SourcePort)},
			Target: &upgrade.Segment{request.TargetBinDir, segment.TargetDataDir, dbid, int(segment.TargetPort)},
		}

		options := []upgrade.Option{
			upgrade.WithExecCommand(execCommand),
			upgrade.WithWorkDir(segment.WorkDir),
			upgrade.WithSegmentMode(),
		}
		if request.CheckOnly {
			options = append(options, upgrade.WithCheckOnly())
		}

		if request.UseLinkMode {
			options = append(options, upgrade.WithLinkMode())
		}

		content := segment.Content
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := upgrade.Run(segmentPair, options...)
			if err != nil {
				failedAction := "upgrade"
				if request.CheckOnly {
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
