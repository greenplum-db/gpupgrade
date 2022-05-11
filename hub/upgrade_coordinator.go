// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
		Mode:          idl.PgOptions_Dispatcher,
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

	// Buffer stdout to add context to errors.
	stdout := new(bytes.Buffer)
	tee := io.MultiWriter(streams.Stdout(), stdout)

	runErr := upgrade.Run(tee, streams.Stderr(), opts)
	if runErr != nil {
		// For "fatal" errors add additional error context. This is useful for customers to see and understand
		// pg_upgrade --check errors.
		var text []string
		var addText bool

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "fatal") || addText {
				addText = true
				text = append(text, line)
			}
		}
		errText := strings.Join(text, "\n")
		if errText == "" {
			return xerrors.Errorf("%s master: %v", action, runErr)
		}

		upgradeDir, err := utils.GetPgUpgradeDir(opts.GetRole(), opts.GetContentID())
		if err != nil {
			runErr = errorlist.Append(runErr, err)
		}

		files, err := filesInDirectory(upgradeDir)
		if err != nil {
			runErr = errorlist.Append(runErr, err)
		}

		for _, file := range files {
			// include the full path of any pg_upgrade error files
			errText = strings.ReplaceAll(errText, file, filepath.Join(upgradeDir, file))
		}

		nextAction := `If you haven't run pre-initialize data migration scripts at the start, please run them.
Consult the gpupgrade documentation for details on the pg_upgrade check error.`
		return utils.NewNextActionErr(xerrors.Errorf("%s master: %s\n\n%v", action, runErr, errText), nextAction)
	}

	return nil
}

// filesInDirectory returns a list of all filenames under the given root.
func filesInDirectory(root string) ([]string, error) {
	entries, err := ioutil.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		files = append(files, entry.Name())
	}

	return files, nil
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
