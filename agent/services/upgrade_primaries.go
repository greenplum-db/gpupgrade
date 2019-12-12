package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

// Allow exec.Command to be mocked out by exectest.NewCommand.
var execCommand = exec.Command

func (s *AgentServer) AgentExecuteUpgradePrimariesSubStep(ctx context.Context, in *idl.UpgradePrimariesRequest) (*idl.UpgradePrimariesReply, error) {
	gplog.Info("agent starting %s", upgradestatus.UPGRADE_PRIMARIES)

	err := UpgradePrimaries(in.OldBinDir, in.NewBinDir, in.DataDirPairs, s.conf.StateDir, in.CheckOnly)
	return &idl.UpgradePrimariesReply{}, err
}

type Segment struct {
	*idl.DataDirPair

	WorkDir string // the pg_upgrade working directory, where logs are stored
}

func UpgradePrimaries(sourceBinDir string, targetBinDir string, dataDirPairs []*idl.DataDirPair, stateDir string, checkOnly bool) error {
	segments := make([]Segment, 0, len(dataDirPairs))

	for _, dataPair := range dataDirPairs {
		err := utils.System.MkdirAll(dataPair.NewDataDir, 0700)
		if err != nil {
			gplog.Error("failed to create segment data directory due to: %v", err)
			return err
		}

		workdir := utils.SegmentPGUpgradeDirectory(stateDir, int(dataPair.Content))
		err = utils.System.MkdirAll(workdir, 0700)
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
	// TODO: consolidate this logic with Hub.ConvertMaster().

	host, err := os.Hostname()
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	agentErrs := make(chan error, len(segments))
	path := filepath.Join(targetBinDir, "pg_upgrade")

	for _, segment := range segments {
		dbid := strconv.Itoa(int(segment.DBID))
		args := []string{
			"--old-bindir", sourceBinDir,
			"--old-datadir", segment.OldDataDir,
			"--old-port", strconv.Itoa(int(segment.OldPort)),
			"--old-gp-dbid", dbid,
			"--new-bindir", targetBinDir,
			"--new-datadir", segment.NewDataDir,
			"--new-port", strconv.Itoa(int(segment.NewPort)),
			"--new-gp-dbid", dbid,
			"--mode=segment",
			"--retain",
		}
		if checkOnly {
			args = append(args, "--check")
		}
		cmd := execCommand(path, args...)

		cmd.Dir = segment.WorkDir

		// Explicitly clear the child environment. pg_upgrade shouldn't need things
		// like PATH, and PGPORT et al are explicitly forbidden to be set.
		cmd.Env = []string{}

		// XXX ...but we make a single exception for now, for LD_LIBRARY_PATH, to
		// work around pervasive problems with RPATH settings in our Postgres
		// extension modules.
		if path, ok := os.LookupEnv("LD_LIBRARY_PATH"); ok {
			cmd.Env = append(cmd.Env, fmt.Sprintf("LD_LIBRARY_PATH=%s", path))
		}

		content := segment.Content
		wg.Add(1)
		go func() {
			defer wg.Done()

			_, err := cmd.Output()
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
