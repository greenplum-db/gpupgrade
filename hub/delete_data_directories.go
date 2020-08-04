// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"golang.org/x/net/context"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
)

func DeleteMirrorAndStandbyDataDirectories(agentConns []*Connection, cluster *greenplum.Cluster) error {
	segs := cluster.SelectSegments(func(seg *greenplum.SegConfig) bool {
		return seg.Role == greenplum.MirrorRole
	})
	return deleteDataDirectories(agentConns, segs)
}

func DeletePrimaryDataDirectories(agentConns []*Connection, segConfigs greenplum.SegConfigs) error {
	return deleteDataDirectories(agentConns, segConfigs)
}

func deleteDataDirectories(agentConns []*Connection, segConfigs greenplum.SegConfigs) error {
	request := func(conn *Connection) error {

		segs := segConfigs.Select(func(seg *greenplum.SegConfig) bool {
			return seg.Hostname == conn.Hostname
		})

		if len(segs) == 0 {
			// This can happen if there are no segments matching the filter on a host
			return nil
		}

		req := new(idl.DeleteDataDirectoriesRequest)
		for _, seg := range segs {
			datadir := seg.DataDir
			req.Datadirs = append(req.Datadirs, datadir)
		}

		_, err := conn.AgentClient.DeleteDataDirectories(context.Background(), req)
		return err
	}

	return ExecuteRPC(agentConns, request)
}
