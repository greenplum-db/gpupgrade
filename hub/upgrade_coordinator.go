// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

func UpgradeCoordinator(streams step.OutStreams, source *greenplum.Cluster, intermediate *greenplum.Cluster, action idl.PgOptions_Action, linkMode bool) error {
	oldOptions := ""
	// When upgrading from 5 the coordinator must be provided with its standby's dbid to allow WAL to sync.
	if source.Version.Major == 5 && source.HasStandby() {
		oldOptions = fmt.Sprintf("-x %d", source.Standby().DbID)
	}

	opts := &idl.PgOptions{
		Action:        action,
		Role:          intermediate.Coordinator().Role,
		ContentID:     int32(intermediate.Coordinator().ContentID),
		Mode:          idl.PgOptions_dispatcher,
		OldOptions:    oldOptions,
		LinkMode:      linkMode,
		TargetVersion: intermediate.Version.String(),
		OldBinDir:     filepath.Join(source.GPHome, "bin"),
		OldDataDir:    source.CoordinatorDataDir(),
		OldPort:       strconv.Itoa(source.CoordinatorPort()),
		OldDBID:       strconv.Itoa(int(source.Coordinator().DbID)),
		NewBinDir:     filepath.Join(intermediate.GPHome, "bin"),
		NewDataDir:    intermediate.CoordinatorDataDir(),
		NewPort:       strconv.Itoa(intermediate.CoordinatorPort()),
		NewDBID:       strconv.Itoa(int(intermediate.Coordinator().DbID)),
	}

	err := RsyncCoordinatorDataDir(streams, utils.GetCoordinatorPreUpgradeBackupDir(), intermediate.CoordinatorDataDir())
	if err != nil {
		return err
	}

	err = upgrade.Run(streams.Stdout(), streams.Stderr(), opts)
	if err != nil {
		if opts.Action != idl.PgOptions_check {
			return xerrors.Errorf("%s master: %v", action, err)
		}

		dir, dirErr := utils.GetPgUpgradeDir(opts.GetRole(), opts.GetContentID())
		if dirErr != nil {
			err = errorlist.Append(err, dirErr)
		}

		nextAction := fmt.Sprintf(`Consult the pg_upgrade check output files located: %s
Refer to the gpupgrade documentation for details on the pg_upgrade check error.

If you haven't already run the pre-initialize data migration scripts please run them.

To connect to the intermedaite target cluster:
source %s
MASTER_DATA_DIRECTORY=%s
PGPORT=%d`, dir, filepath.Join(intermediate.GPHome, "greenplum_path.sh"), intermediate.CoordinatorDataDir(), intermediate.CoordinatorPort())
		return utils.NewNextActionErr(xerrors.Errorf("%s master: %v", action, err), nextAction)
	}

	return nil
}

func RsyncCoordinatorDataDir(stream step.OutStreams, sourceDir, targetDir string) error {
	sourceDirRsync := filepath.Clean(sourceDir) + string(os.PathSeparator)

	options := []rsync.Option{
		rsync.WithSources(sourceDirRsync),
		rsync.WithDestination(targetDir),
		rsync.WithOptions("--archive", "--delete"),
		rsync.WithExcludedFiles("pg_log/*"),
		rsync.WithStream(stream),
	}

	err := rsync.Rsync(options...)
	if err != nil {
		return xerrors.Errorf("rsync %q to %q: %w", sourceDirRsync, targetDir, err)
	}

	return nil
}
