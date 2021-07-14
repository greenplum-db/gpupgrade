// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"

	"github.com/greenplum-db/gpupgrade/idl"
)

func DeleteStateDirectories(agentConns []*idl.Connection, excludeHostname string) error {
	request := func(conn *idl.Connection) error {
		if conn.Hostname == excludeHostname {
			return nil
		}

		_, err := conn.AgentClient.DeleteStateDirectory(context.Background(), &idl.DeleteStateDirectoryRequest{})
		return err
	}

	return ExecuteRPC(agentConns, request)
}
