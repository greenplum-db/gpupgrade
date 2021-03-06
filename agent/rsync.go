// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"os"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

func (s *Server) RsyncDataDirectories(ctx context.Context, in *idl.RsyncRequest) (*idl.RsyncReply, error) {
	gplog.Info("agent received request to rsync data directories")

	// verify source data directories
	var mErr error
	for _, pair := range in.Pairs {
		err := upgrade.VerifyDataDirectory(pair.GetSource())
		if err != nil {
			mErr = errorlist.Append(mErr, err)
		}
	}
	if mErr != nil {
		return &idl.RsyncReply{}, mErr
	}

	return &idl.RsyncReply{}, rsyncRequestDirs(in)
}

func (s *Server) RsyncTablespaceDirectories(ctx context.Context, in *idl.RsyncRequest) (*idl.RsyncReply, error) {
	gplog.Info("agent received request to rsync tablespace directories")

	// We can only verify the source directories since the destination
	// directories are on another host.
	var sourceDirs []string
	for _, pair := range in.Pairs {
		sourceDirs = append(sourceDirs, pair.GetSource())
	}

	// NOTE: Rsync will still be called if a given sourceDir is empty.
	if err := upgrade.Verify5XTablespaceDirectories(sourceDirs); err != nil {
		return &idl.RsyncReply{}, err
	}

	return &idl.RsyncReply{}, rsyncRequestDirs(in)
}

func rsyncRequestDirs(in *idl.RsyncRequest) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(in.Pairs))

	for _, pair := range in.Pairs {
		pair := pair

		wg.Add(1)
		go func() {
			defer wg.Done()

			opts := []rsync.Option{
				rsync.WithSources(pair.GetSource() + string(os.PathSeparator)),
				rsync.WithDestinationHost(pair.GetDestinationHost()),
				rsync.WithDestination(pair.GetDestination()),
				rsync.WithOptions(in.GetOptions()...),
				rsync.WithExcludedFiles(in.GetExcludes()...),
			}
			errs <- rsync.Rsync(opts...)
		}()
	}

	wg.Wait()
	close(errs)

	var err error
	for e := range errs {
		err = errorlist.Append(err, e)
	}

	return err
}
