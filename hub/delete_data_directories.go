// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/net/context"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
)

func DeleteMirrorAndStandbyDataDirectories(agentConns []*Connection, cluster *greenplum.Cluster) error {
	return deleteDataDirectories(agentConns, cluster, false)
}

func DeletePrimaryDataDirectories(agentConns []*Connection, cluster *greenplum.Cluster) error {
	return deleteDataDirectories(agentConns, cluster, true)
}

func deleteDataDirectories(agentConns []*Connection, cluster *greenplum.Cluster, primaries bool) error {
	wg := sync.WaitGroup{}
	errChan := make(chan error, len(agentConns))

	for _, conn := range agentConns {
		conn := conn

		filterFunc := func(seg *greenplum.SegConfig) bool {
			if seg.Hostname != conn.Hostname {
				return false
			}

			if primaries {
				return seg.IsPrimary()
			}
			return seg.Role == greenplum.MirrorRole
		}

		segments := cluster.SelectSegments(filterFunc)
		if len(segments) == 0 {
			// This can happen if there are no segments matching the filter on a host
			continue
		}

		wg.Add(1)
		go func(c *Connection) {
			defer wg.Done()

			req := new(idl.DeleteDataDirectoriesRequest)
			for _, seg := range segments {
				datadir := seg.DataDir
				req.Datadirs = append(req.Datadirs, datadir)
			}

			_, err := c.AgentClient.DeleteDataDirectories(context.Background(), req)
			if err != nil {
				gplog.Error("Error deleting data directories on host %s: %s",
					c.Hostname, err.Error())
				errChan <- err
			}
		}(conn)
	}

	wg.Wait()
	close(errChan)

	var mErr *multierror.Error
	for err := range errChan {
		mErr = multierror.Append(mErr, err)
	}

	return mErr.ErrorOrNil()
}
