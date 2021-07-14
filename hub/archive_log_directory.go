// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"

	"github.com/greenplum-db/gpupgrade/idl"
)

func ArchiveSegmentLogDirectories(agentConns []*idl.Connection, excludeHostname, newDir string) error {
	request := func(conn *idl.Connection) error {
		if conn.Hostname == excludeHostname {
			return nil
		}

		_, err := conn.AgentClient.ArchiveLogDirectory(context.Background(), &idl.ArchiveLogDirectoryRequest{
			NewDir: newDir,
		})
		return err
	}

	return ExecuteRPC(agentConns, request)
}
