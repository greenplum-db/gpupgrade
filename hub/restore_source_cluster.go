// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

var RecoversegCmd = exec.Command

var Options = []string{"--archive", "--compress", "--stats"}

var Excludes = []string{
	"pg_hba.conf", "postmaster.opts", "postgresql.auto.conf", "internal.auto.conf",
	"gp_dbid", "postgresql.conf", "backup_label.old", "postmaster.pid", "recovery.conf",
}

func RsyncMasterAndPrimaries(stream step.OutStreams, agentConns []*Connection, source *greenplum.Cluster) error {
	if !source.HasAllMirrorsAndStandby() {
		return errors.New("Source cluster does not have mirrors and/or standby. Cannot restore source cluster. Please contact support.")
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- RsyncMaster(stream, source.Standby(), source.Master())
	}()

	errs <- RsyncPrimaries(agentConns, source)

	wg.Wait()
	close(errs)

	var mErr *multierror.Error
	for err := range errs {
		mErr = multierror.Append(mErr, err)
	}

	return mErr.ErrorOrNil()
}

// Restoring the mirrors is needed in copy mode on 5X since the source cluster
// is left in a bad state after execute. This is because running pg_upgrade on
// a primary results in a checkpoint that does not get replicated on the mirror.
// Thus, when the mirror is started it panics and a gprecoverseg or rsync is needed.
func Recoverseg(stream step.OutStreams, cluster *greenplum.Cluster) error {
	if cluster.Version.AtLeast("6") {
		return nil
	}

	script := fmt.Sprintf("source %[1]s/greenplum_path.sh && %[1]s/bin/gprecoverseg -a", cluster.GPHome)
	cmd := RecoversegCmd("bash", "-c", script)

	cmd.Stdout = stream.Stdout()
	cmd.Stderr = stream.Stderr()

	gplog.Info("running command: %q", cmd)
	return cmd.Run()
}

func RsyncMaster(stream step.OutStreams, standby greenplum.SegConfig, master greenplum.SegConfig) error {
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

func RsyncPrimaries(agentConns []*Connection, source *greenplum.Cluster) error {
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
