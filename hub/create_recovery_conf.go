// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func CreateRecoveryConfOnSegments(agentConns []*idl.Connection, intermediate *greenplum.Cluster) error {
	user, err := utils.System.Current()
	if err != nil {
		return err
	}

	request := func(conn *idl.Connection) error {
		intermediateMirrors := intermediate.SelectSegments(func(seg *greenplum.SegConfig) bool {
			return seg.IsOnHost(conn.Hostname) && !seg.IsStandby() && seg.IsMirror()
		})

		var connReqs []*idl.CreateRecoveryConfRequest_Connection
		for _, intermediateMirror := range intermediateMirrors {
			intermediatePrimary := intermediate.Primaries[intermediateMirror.ContentID]

			connReq := &idl.CreateRecoveryConfRequest_Connection{
				MirrorDataDir: intermediateMirror.DataDir,
				User:          user.Username,
				PrimaryHost:   intermediatePrimary.Hostname,
				PrimaryPort:   int32(intermediatePrimary.Port),
			}

			connReqs = append(connReqs, connReq)
		}

		req := &idl.CreateRecoveryConfRequest{Connections: connReqs}
		_, err := conn.AgentClient.CreateRecoveryConf(context.Background(), req)
		return err
	}

	return ExecuteRPC(agentConns, request)
}
