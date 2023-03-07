// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/blang/semver/v4"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

type Result struct {
	stdout bytes.Buffer
	stderr bytes.Buffer
	err    error
}

func Copy(streams step.OutStreams, sourceDirs []string, agentHostsToBackupDir AgentHostsToBackupDir) error {
	/*
	 * Copy the directories once per host.
	 */
	var wg sync.WaitGroup

	results := make(chan *Result, len(agentHostsToBackupDir))

	for hostname, backupDir := range agentHostsToBackupDir {

		wg.Add(1)
		go func(hostname string, backupDir string) {
			defer wg.Done()

			stream := &step.BufferedStreams{}

			options := []rsync.Option{
				rsync.WithSources(sourceDirs...),
				rsync.WithDestinationHost(hostname),
				rsync.WithDestination(backupDir),
				rsync.WithOptions("--archive", "--compress", "--delete", "--stats"),
				rsync.WithStream(stream),
			}

			err := rsync.Rsync(options...)
			if err != nil {
				err = xerrors.Errorf("copying source %q to destination %q on host %s: %w", sourceDirs, backupDir, hostname, err)
			}
			result := Result{stdout: stream.StdoutBuf, stderr: stream.StderrBuf, err: err}
			results <- &result
		}(hostname, backupDir)
	}

	wg.Wait()
	close(results)

	var errs error

	for result := range results {
		if _, err := io.Copy(streams.Stdout(), &result.stdout); err != nil {
			errs = errorlist.Append(errs, err)
		}

		if _, err := io.Copy(streams.Stderr(), &result.stderr); err != nil {
			errs = errorlist.Append(errs, err)
		}

		if result.err != nil {
			errs = errorlist.Append(errs, result.err)
		}
	}

	return errs
}

func CopyCoordinatorDataDir(streams step.OutStreams, coordinatorDataDir string, agentHostsToBackupDir AgentHostsToBackupDir) error {
	// Make sure sourceDir ends with a trailing slash so that rsync will
	// transfer the directory contents and not the directory itself.
	source := []string{filepath.Clean(coordinatorDataDir) + string(filepath.Separator)}

	destinationHostToBackupDir := make(AgentHostsToBackupDir)
	for host, backupDir := range agentHostsToBackupDir {
		destinationHostToBackupDir[host] = utils.GetCoordinatorPostUpgradeBackupDir(backupDir)
	}

	return Copy(streams, source, destinationHostToBackupDir)
}

func CopyCoordinatorTablespaces(streams step.OutStreams, sourceVersion semver.Version, tablespaces greenplum.Tablespaces, agentHostsToBackupDir AgentHostsToBackupDir) error {
	if tablespaces == nil && sourceVersion.Major != 5 {
		return nil
	}

	var sourcePaths []string
	if sourceVersion.Major == 5 {
		// 5X always needs to include the --old-tablespaces-file
		sourcePaths = append(sourcePaths, utils.GetStateDirOldTablespacesFile())
	}

	sourcePaths = append(sourcePaths, tablespaces.GetCoordinatorTablespaces().UserDefinedTablespacesLocations()...)

	destinationHostToBackupDir := make(AgentHostsToBackupDir)
	for host, backupDir := range agentHostsToBackupDir {
		// ensure the destination backup directory has a trailing slash so rsync
		// will transfer the directory contents and not the directory itself.
		destinationHostToBackupDir[host] = utils.GetTablespaceBackupDir(backupDir) + string(os.PathSeparator)
	}

	return Copy(streams, sourcePaths, destinationHostToBackupDir)
}
