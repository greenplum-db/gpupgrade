// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"fmt"
	"sync"

	multierror "github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils/disk"
)

func (s *Server) CheckDiskSpace(ctx context.Context, in *idl.CheckDiskSpaceRequest) (*idl.CheckDiskSpaceReply, error) {
	reply := new(idl.CheckDiskSpaceReply)

	agents, err := s.AgentConns()
	if err != nil {
		return reply, err
	}

	reply.Failed, err = checkDiskSpace(ctx, s.Source, agents, disk.Local, in)
	return reply, err
}

func checkDiskSpace(ctx context.Context, cluster *greenplum.Cluster, agents []*Connection, d disk.Disk, in *idl.CheckDiskSpaceRequest) (disk.SpaceFailures, error) {
	var wg sync.WaitGroup
	errs := make(chan error, len(agents)+1)
	failures := make(chan disk.SpaceFailures, len(agents)+1)

	wg.Add(1)
	go func() {
		defer wg.Done()

		failed, err := disk.CheckUsage(d, in.Ratio, cluster.MasterDataDir())
		if err != nil {
			errs <- xerrors.Errorf("check disk space on master host: %w", err)
		}

		if len(failed) > 0 {
			masterHost := cluster.GetHostForContent(-1)
			failures <- prefixWith(masterHost, failed)
		}
	}()

	for i := range agents {
		agent := agents[i]
		wg.Add(1)

		// We want to check disk space for the standby, primaries, and mirrors.
		excludingMaster := func(seg *greenplum.SegConfig) bool {
			return seg.IsOnHost(agent.Hostname) && !seg.IsMaster()
		}

		go func() {
			defer wg.Done()

			segments := cluster.SelectSegments(excludingMaster)
			if len(segments) == 0 {
				errs <- greenplum.UnknownHostError{Hostname: agent.Hostname}
				return
			}

			req := &idl.CheckSegmentDiskSpaceRequest{
				Request: in,
			}
			for _, s := range segments {
				req.Datadirs = append(req.Datadirs, s.DataDir)
			}

			reply, err := agent.AgentClient.CheckDiskSpace(ctx, req)
			if err != nil {
				errs <- xerrors.Errorf("check disk space on host %s: %w", agent.Hostname, err)
				return
			}

			if len(reply.Failed) > 0 {
				// Because different hosts can have identical paths for their
				// data directories, make sure every failure is uniquely
				// identified by its hostname.
				failures <- prefixWith(agent.Hostname, reply.Failed)
			}
		}()
	}

	wg.Wait()
	close(errs)
	close(failures)

	var multiErr *multierror.Error
	for err := range errs {
		multiErr = multierror.Append(multiErr, err)
	}
	if err := multiErr.ErrorOrNil(); err != nil {
		return nil, err
	}

	result := make(disk.SpaceFailures)
	for failure := range failures {
		for k, v := range failure {
			result[k] = v
		}
	}
	return result, nil
}

// prefixWith adds a string prefix to every key in the failure map.
func prefixWith(prefix string, failures disk.SpaceFailures) disk.SpaceFailures {
	prefixed := make(disk.SpaceFailures)
	for k, v := range failures {
		newKey := fmt.Sprintf("%s: %s", prefix, k)
		prefixed[newKey] = v
	}
	return prefixed
}
