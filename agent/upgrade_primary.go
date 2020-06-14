// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

func upgradeSegment(segment Segment, request *idl.UpgradePrimariesRequest, host string) error {
	err := restoreBackup(request, segment)

	if err != nil {
		return xerrors.Errorf("restore master data directory backup on host %s for content id %d: %w",
			host, segment.Content, err)
	}

	err = RestoreTablespaces(request, segment)
	if err != nil {
		return xerrors.Errorf("restore tablespace on host %s for content id %d: %w",
			host, segment.Content, err)
	}

	err = performUpgrade(segment, request)

	if err != nil {
		failedAction := "upgrade"
		if request.CheckOnly {
			failedAction = "check"
		}
		return xerrors.Errorf("%s primary on host %s with content %d: %w", failedAction, host, segment.Content, err)
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
	} else {
		// During gpupgrade execute, tablepace mapping file is copied after
		// the master has been upgraded. So, don't pass this option during
		// --check mode. There is no test in pg_upgrade which depends on the
		// existence of this file.
		options = append(options, upgrade.WithTablespaceFile(request.TablespacesMappingFilePath))
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

	options := []rsync.Option{
		rsync.WithSources(request.MasterBackupDir + string(os.PathSeparator)),
		rsync.WithDestination(segment.TargetDataDir),
		rsync.WithOptions("--archive", "--delete"),
		rsync.WithExcludedFiles(
			"internal.auto.conf",
			"postgresql.conf",
			"pg_hba.conf",
			"postmaster.opts",
			"gp_dbid",
			"gpssh.conf",
			"gpperfmon"),
	}

	return rsync.Rsync(options...)
}

func RestoreTablespaces(request *idl.UpgradePrimariesRequest, segment Segment) error {
	if request.CheckOnly {
		return nil
	}

	for oid, tablespace := range segment.Tablespaces {
		if !tablespace.GetUserDefined() {
			continue
		}

		targetDir := greenplum.GetTablespaceLocationForDbId(tablespace, int(segment.DBID))
		sourceDir := greenplum.GetMasterTablespaceLocation(filepath.Dir(request.TablespacesMappingFilePath), int(oid))

		options := []rsync.Option{
			rsync.WithSources(sourceDir + string(os.PathSeparator)),
			rsync.WithDestination(targetDir),
			rsync.WithOptions("--archive", "--delete"),
		}

		if err := rsync.Rsync(options...); err != nil {
			return xerrors.Errorf("rsync master tablespace directory to segment tablespace directory: %w", err)
		}

		symLinkName := fmt.Sprintf("%s/pg_tblspc/%s", segment.TargetDataDir, strconv.Itoa(int(oid)))
		if err := ReCreateSymLink(targetDir, symLinkName); err != nil {
			return xerrors.Errorf("recreate symbolic link: %w", err)
		}
	}

	return nil
}

var ReCreateSymLink = func(sourceDir, symLinkName string) error {
	return reCreateSymLink(sourceDir, symLinkName)
}

func reCreateSymLink(sourceDir, symLinkName string) error {
	_, err := utils.System.Lstat(symLinkName)
	if err == nil {
		if err := utils.System.Remove(symLinkName); err != nil {
			return xerrors.Errorf("unlink %q: %w", symLinkName, err)
		}
	} else if !os.IsNotExist(err) {
		return xerrors.Errorf("stat symbolic link %q: %w", symLinkName, err)
	}

	if err := utils.System.Symlink(sourceDir, symLinkName); err != nil {
		return xerrors.Errorf("create symbolic link %q to directory %q: %w", symLinkName, sourceDir, err)
	}

	return nil
}
