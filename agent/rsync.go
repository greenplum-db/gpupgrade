// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

func (s *Server) RsyncDataDirectories(ctx context.Context, in *idl.RsyncRequest) (*idl.RsyncReply, error) {
	gplog.Info("agent received request to rsync data directories")

	// verify source data directories
	var mErr error
	for _, opts := range in.GetOptions() {
		err := upgrade.VerifyDataDirectory(opts.GetSources()...)
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
	for _, opts := range in.GetOptions() {
		for _, dir := range opts.GetSources() {
			if err := upgrade.VerifyTablespaceLocation(utils.System.DirFS(dir), dir); err != nil {
				return &idl.RsyncReply{}, err
			}
		}
	}

	return &idl.RsyncReply{}, rsyncRequestDirs(in)
}

func rsyncRequestDirs(in *idl.RsyncRequest) error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(in.GetOptions()))

	for _, opts := range in.GetOptions() {
		opts := opts

		wg.Add(1)
		go func() {
			defer wg.Done()

			opts := []rsync.Option{
				rsync.WithSources(opts.GetSources()...),
				rsync.WithDestinationHost(opts.GetDestinationHost()),
				rsync.WithDestination(opts.GetDestination()),
				rsync.WithOptions(opts.GetOptions()...),
				rsync.WithExcludedFiles(opts.GetExcludedFiles()...),
			}
			err := rsync.Rsync(opts...)
			if err != nil {
				errs <- fmt.Errorf("on host %q: %w", hostname, err)
			}
		}()
	}

	wg.Wait()
	close(errs)

	for e := range errs {
		err = errorlist.Append(err, e)
	}

	return err
}
