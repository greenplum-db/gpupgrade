// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"os"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

var Options = []string{"--archive", "--compress", "--stats"}

var Excludes = []string{
	"pg_hba.conf", "postmaster.opts", "postgresql.auto.conf", "internal.auto.conf",
	"gp_dbid", "postgresql.conf", "backup_label.old", "postmaster.pid", "recovery.conf",
}

func RestoreMasterAndPrimaries(stream step.OutStreams, agentConns []*Connection, source *greenplum.Cluster) error {
	if !source.HasAllMirrorsAndStandby() {
		return errors.New("Source cluster does not have mirrors and/or standby. Cannot restore source cluster. Please contact support.")
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- RestoreMaster(stream, source.Standby(), source.Master())
	}()

	errs <- RestorePrimaries(agentConns, source)

	wg.Wait()
	close(errs)

	var mErr *multierror.Error
	for err := range errs {
		mErr = multierror.Append(mErr, err)
	}

	return mErr.ErrorOrNil()
}

func RestoreMaster(stream step.OutStreams, standby greenplum.SegConfig, master greenplum.SegConfig) error {
	opts := []rsync.Option{
		rsync.WithSources(standby.DataDir + string(os.PathSeparator)),
		rsync.WithRemoteHost(master.Hostname),
		rsync.WithDestination(master.DataDir),
		rsync.WithOptions(Options...),
		rsync.WithExcludedFiles(Excludes...),
		rsync.WithStream(stream),
	}

	return rsync.Rsync(opts...)
}

func RestorePrimaries(agentConns []*Connection, source *greenplum.Cluster) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(agentConns))

	for _, conn := range agentConns {
		conn := conn

		wg.Add(1)
		go func() {
			defer wg.Done()

			mirrors := source.SelectSegments(func(seg *greenplum.SegConfig) bool {
				return seg.IsOnHost(conn.Hostname) && !seg.IsStandby() && seg.IsMirror()
			})
			if len(mirrors) == 0 {
				return
			}

			var pairs []*idl.RsyncPair
			for _, mirror := range mirrors {
				pair := &idl.RsyncPair{
					Source:      mirror.DataDir + string(os.PathSeparator),
					RemoteHost:  source.Primaries[mirror.ContentID].Hostname,
					Destination: source.Primaries[mirror.ContentID].DataDir,
				}
				pairs = append(pairs, pair)
			}

			req := &idl.RsyncRequest{
				Options:  Options,
				Excludes: Excludes,
				Pairs:    pairs,
			}

			_, err := conn.AgentClient.Rsync(context.Background(), req)
			errs <- err
		}()
	}

	wg.Wait()
	close(errs)

	var mErr *multierror.Error
	for err := range errs {
		mErr = multierror.Append(mErr, err)
	}

	return mErr.ErrorOrNil()
}
