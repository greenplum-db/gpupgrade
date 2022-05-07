// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"

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

func Copy(streams step.OutStreams, destinationDir string, sourceDirs, hosts []string) error {
	/*
	 * Copy the directories once per host.
	 */
	var wg sync.WaitGroup

	results := make(chan *Result, len(hosts))

	for _, hostname := range hosts {
		hostname := hostname // capture range variable

		wg.Add(1)
		go func() {
			defer wg.Done()

			stream := &step.BufferedStreams{}

			options := []rsync.Option{
				rsync.WithSources(sourceDirs...),
				rsync.WithDestinationHost(hostname),
				rsync.WithDestination(destinationDir),
				rsync.WithOptions("--archive", "--compress", "--delete", "--stats"),
				rsync.WithStream(stream),
			}

			err := rsync.Rsync(options...)
			if err != nil {
				err = xerrors.Errorf("copying source %q to destination %q on host %s: %w", sourceDirs, destinationDir, hostname, err)
			}
			result := Result{stdout: stream.StdoutBuf, stderr: stream.StderrBuf, err: err}
			results <- &result
		}()
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

func CopyCoordinatorDataDir(streams step.OutStreams, coordinatorDataDir string, destination string, hosts []string) error {
	// Make sure sourceDir ends with a trailing slash so that rsync will
	// transfer the directory contents and not the directory itself.
	source := []string{filepath.Clean(coordinatorDataDir) + string(filepath.Separator)}
	return Copy(streams, destination, source, hosts)
}

func CopyCoordinatorTablespaces(streams step.OutStreams, tablespaces greenplum.Tablespaces, destinationDir string, hosts []string) error {
	if tablespaces == nil {
		return nil
	}

	// include tablespace mapping file which is used as a parameter to pg_upgrade
	sourcePaths := []string{utils.GetTablespaceMappingFile()}
	sourcePaths = append(sourcePaths, tablespaces.GetCoordinatorTablespaces().UserDefinedTablespacesLocations()...)

	return Copy(streams, destinationDir+string(os.PathSeparator), sourcePaths, hosts)
}
