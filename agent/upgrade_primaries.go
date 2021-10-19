// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"os"
	"os/exec"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func (s *Server) UpgradePrimaries(ctx context.Context, request *idl.UpgradePrimariesRequest) (*idl.UpgradePrimariesReply, error) {
	if request.CheckOnly {
		gplog.Info("agent starting %s", idl.Substep_CHECK_UPGRADE)
	} else {
		gplog.Info("agent starting %s", idl.Substep_UPGRADE_PRIMARIES)
	}

	err := UpgradePrimaries(request)

	return &idl.UpgradePrimariesReply{}, err
}

// Allow exec.Command to be mocked out by exectest.NewCommand.
var execCommand = exec.Command

type Segment struct {
	*idl.DataDirPair

	WorkDir string // the pg_upgrade working directory, where logs are stored
}

func UpgradePrimaries(request *idl.UpgradePrimariesRequest) error {
	segments, err := buildSegments(request)

	if err != nil {
		return err
	}

	host, err := os.Hostname()
	if err != nil {
		return err
	}

	//
	// Upgrade each segment concurrently
	//
	upgradeResponse := make(chan error, len(segments))

	for _, segment := range segments {
		segment := segment // capture the range variable

		go func() {
			upgradeResponse <- upgradeSegment(segment, request, host)
		}()
	}

	for range segments {
		response := <-upgradeResponse
		if response != nil {
			err = errorlist.Append(err, response)
		}
	}

	//
	// Collect and handle errors
	//
	if err != nil {
		failedAction := "upgrade"
		if request.CheckOnly {
			failedAction = "check"
		}
		return xerrors.Errorf("%s primaries: %w", failedAction, err)
	}

	// success
	return nil
}

func buildSegments(request *idl.UpgradePrimariesRequest) ([]Segment, error) {
	segments := make([]Segment, 0, len(request.DataDirPairs))

	for _, dataPair := range request.DataDirPairs {
		workdir, err := utils.GetPgUpgradeDir(greenplum.PrimaryRole, int(dataPair.Content))
		if err != nil {
			return nil, err
		}

		err = utils.System.MkdirAll(workdir, 0700)
		if err != nil {
			return nil, xerrors.Errorf("creating pg_upgrade work directory: %w", err)
		}

		segments = append(segments, Segment{
			DataDirPair: dataPair,
			WorkDir:     workdir,
		})
	}

	return segments, nil
}
