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

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

// Allow exec.Command to be mocked out by exectest.NewCommand.
var execCommand = exec.Command

func (s *AgentServer) AgentExecuteUpgradePrimariesSubStep(ctx context.Context, in *idl.UpgradePrimariesRequest) (*idl.UpgradePrimariesReply, error) {
	gplog.Info("agent starting %s", upgradestatus.UPGRADE_PRIMARIES)

	err := UpgradePrimaries(in.OldBinDir, in.NewBinDir, in.DataDirPairs)
	return &idl.UpgradePrimariesReply{}, err
}

type Segment struct {
	*idl.DataDirPair
	UpgradeDir string
}

func UpgradePrimaries(sourceBinDir string, targetBinDir string, dataDirPairs []*idl.DataDirPair) error {
	segments := make([]Segment, 0, len(dataDirPairs))

	for _, dataPair := range dataDirPairs {
		err := utils.System.MkdirAll(dataPair.NewDataDir, 0700)
		if err != nil {
			gplog.Error("failed to create segment data directory due to: %v", err)
			return err
		}

		segments = append(segments, Segment{DataDirPair: dataPair, UpgradeDir: dataPair.NewDataDir})
	}

	err := UpgradeSegments(sourceBinDir, targetBinDir, segments)
	if err != nil {
		return errors.Wrap(err, "failed to upgrade segments")
	}

	return nil
}

func UpgradeSegments(sourceBinDir string, targetBinDir string, segments []Segment) error {
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
		cmd := execCommand(path,
			"--old-bindir", sourceBinDir,
			"--old-datadir", segment.OldDataDir,
			"--old-port", strconv.Itoa(int(segment.OldPort)),
			"--new-bindir", targetBinDir,
			"--new-datadir", segment.NewDataDir,
			"--new-port", strconv.Itoa(int(segment.NewPort)),
			"--mode=segment",
		)

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

		go func() {
			err := cmd.Run()
			if err != nil {
				agentErrs <- errors.Wrapf(err, "failed to upgrade primary on host %s with content %d", host, segment.Content)
			}

			wg.Done()
		}()
	}

	wg.Wait()
	close(agentErrs)

	for agentErr := range agentErrs {
		err = multierror.Append(err, agentErr)
	}

	return err
}
