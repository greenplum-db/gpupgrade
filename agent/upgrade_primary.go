// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

func upgradeSegment(segment Segment, request *idl.UpgradePrimariesRequest, host string) error {
	err := restoreBackup(request, segment)

	if err != nil {
		return errors.Wrapf(err, "failed to restore master data directory backup on host %s for content id %d: %s",
			host, segment.Content, err)
	}

	err = performUpgrade(segment, request)

	if err != nil {
		failedAction := "upgrade"
		if request.CheckOnly {
			failedAction = "check"
		}
		return errors.Wrapf(err, "failed to %s primary on host %s with content %d", failedAction, host, segment.Content)
	}

	return nil
}

func performUpgrade(segment Segment, request *idl.UpgradePrimariesRequest) error {
	dbid := int(segment.DBID)
	segmentPair := upgrade.SegmentPair{
		Source: &upgrade.Segment{BinDir: request.SourceBinDir, DataDir: segment.SourceDataDir, DBID: dbid, Port: int(segment.SourcePort)},
		Target: &upgrade.Segment{BinDir: request.TargetBinDir, DataDir: segment.TargetDataDir, DBID: dbid, Port: int(segment.TargetPort)},
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

	return upgrade.Run(segmentPair, options...)
}

func restoreBackup(request *idl.UpgradePrimariesRequest, segment Segment) error {
	if request.CheckOnly {
		return nil
	}

	return Rsync(request.MasterBackupDir, segment.TargetDataDir, []string{
		"internal.auto.conf",
		"postgresql.conf",
		"pg_hba.conf",
		"postmaster.opts",
		"gp_dbid",
		"gpssh.conf",
		"gpperfmon",
	})
}
