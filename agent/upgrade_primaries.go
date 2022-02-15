// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

func (s *Server) UpgradePrimaries(ctx context.Context, req *idl.UpgradePrimariesRequest) (*idl.UpgradePrimariesReply, error) {
	gplog.Info("agent starting %s", req.GetAction())

	err := upgradePrimariesInParallel(req.GetOpts())
	if err != nil {
		return &idl.UpgradePrimariesReply{}, err
	}

	return &idl.UpgradePrimariesReply{}, nil
}

func upgradePrimariesInParallel(opts []*idl.PgOptions) error {
	host, err := utils.System.Hostname()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(opts))

	for _, opt := range opts {
		wg.Add(1)
		go func(host string, opt *idl.PgOptions) {
			defer wg.Done()

			errs <- upgradePrimarySegment(host, opt)
		}(host, opt)
	}

	wg.Wait()
	close(errs)

	for e := range errs {
		err = errorlist.Append(err, e)
	}

	return err
}

func upgradePrimarySegment(host string, opt *idl.PgOptions) error {
	if opt.GetAction() != idl.PgOptions_check {
		err := restoreBackup(utils.GetCoordinatorPostUpgradeBackupDir(), opt.GetNewDataDir())
		if err != nil {
			return xerrors.Errorf("restore backup of upgraded master data directory on host %s for content id %d: %w", host, opt.GetContentID(), err)
		}

		err = RestoreTablespaces(opt.GetTablespaces(), opt.GetOldDBID(), opt.GetNewDataDir())
		if err != nil {
			return xerrors.Errorf("restore tablespace on host %s for content id %d: %w", host, opt.GetContentID(), err)
		}
	}

	err := upgrade.Run(ioutil.Discard, ioutil.Discard, opt)
	if err != nil {
		return xerrors.Errorf("%s primary on host %s with content %d: %w", opt.GetAction(), host, opt.GetContentID(), err)
	}

	return nil
}

func restoreBackup(backupDir string, newDataDir string) error {
	options := []rsync.Option{
		rsync.WithSources(backupDir + string(os.PathSeparator)),
		rsync.WithDestination(newDataDir),
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

func RestoreTablespaces(tablespaces map[int32]*idl.TablespaceInfo, oldDBID string, newDataDir string) error {
	dbid, err := strconv.Atoi(oldDBID)
	if err != nil {
		return err
	}

	for oid, tablespace := range tablespaces {
		if !tablespace.GetUserDefined() {
			continue
		}

		targetDir := greenplum.GetTablespaceLocationForDbId(tablespace, dbid)
		sourceDir := greenplum.GetMasterTablespaceLocation(utils.GetTablespaceDir(), int(oid)) + string(os.PathSeparator)

		options := []rsync.Option{
			rsync.WithSources(sourceDir),
			rsync.WithDestination(targetDir),
			rsync.WithOptions("--archive", "--delete"),
		}

		if err := rsync.Rsync(options...); err != nil {
			return xerrors.Errorf("rsync master tablespace directory to segment tablespace directory: %w", err)
		}

		symLinkName := fmt.Sprintf("%s/pg_tblspc/%s", newDataDir, strconv.Itoa(int(oid)))
		if err := recreateSymlink(targetDir, symLinkName); err != nil {
			return xerrors.Errorf("recreate symbolic link: %w", err)
		}
	}

	return nil
}

func recreateSymlink(sourceDir, symLinkName string) error {
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
