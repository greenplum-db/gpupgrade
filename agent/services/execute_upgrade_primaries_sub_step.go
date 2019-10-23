package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

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
	UpgradeDir string
}

func UpgradePrimaries(sourceBinDir string, targetBinDir string, dataDirPairs []*idl.DataDirPair, stateDir string, checkOnly bool) error {
	segments := make([]Segment, 0, len(dataDirPairs))

	for _, dataPair := range dataDirPairs {
		err := utils.System.MkdirAll(dataPair.NewDataDir, 0700)
		if err != nil {
			gplog.Error("failed to create segment data directory due to: %v", err)
			return err
		}

		segments = append(segments, Segment{DataDirPair: dataPair, UpgradeDir: dataPair.NewDataDir})
	}

	err := UpgradeSegments(sourceBinDir, targetBinDir, segments, stateDir, checkOnly)
	if err != nil {
		return errors.Wrap(err, "failed to upgrade segments")
	}

	return nil
}

func UpgradeSegments(sourceBinDir string, targetBinDir string, segments []Segment, stateDir string, checkOnly bool) (err error) {
	// TODO: consolidate this logic with Hub.ConvertMaster().

	host, err := os.Hostname()
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	agentErrs := make(chan error, len(segments))

	for _, segment := range segments {
		wg.Add(1)

		path := filepath.Join(targetBinDir, "pg_upgrade")
		args := []string{
			"--old-bindir", sourceBinDir,
			"--old-datadir", segment.OldDataDir,
			"--old-port", strconv.Itoa(int(segment.OldPort)),
			"--new-bindir", targetBinDir,
			"--new-datadir", segment.NewDataDir,
			"--new-port", strconv.Itoa(int(segment.NewPort)),
			"--mode=segment",
		}
		if checkOnly {
			args = append(args, "--check")
		}
		cmd := execCommand(path, args...)

		cmd.Dir = segment.UpgradeDir

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
		go func() {
			defer wg.Done()

			if checkOnly {
				//TODO: put this in the "official" segment dir location
				stdout, err := utils.System.OpenFile(
					filepath.Join(stateDir, fmt.Sprintf("pg_upgrade_check_stdout_seg_%d.log", content)),
					os.O_WRONLY|os.O_CREATE,
					0600,
				)
				if err != nil {
					agentErrs <- errors.Wrap(err, "could not open stdout log file")
					return
				}
				stderr, err := utils.System.OpenFile(
					filepath.Join(stateDir, fmt.Sprintf("pg_upgrade_check_stderr_seg_%d.log", content)),
					os.O_WRONLY|os.O_CREATE,
					0600,
				)
				if err != nil {
					agentErrs <- errors.Wrap(err, "could not open stderr log file")
					return
				}
				defer func() {
					if closeErr := stdout.Close(); closeErr != nil {
						err = multierror.Append(err,
							xerrors.Errorf("failed to close pg_upgrade_check_stdout log: %w", closeErr))
					}
					if closeErr := stderr.Close(); closeErr != nil {
						err = multierror.Append(err,
							xerrors.Errorf("failed to close pg_upgrade_check_stderr log: %w", closeErr))
					}
				}()

				cmd.Stdout = stdout
				cmd.Stderr = stderr
			}

			err = cmd.Run()
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
