// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"log"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func ArchiveLogDirectories(logArchiveDir string, agentConns []*idl.Connection, targetCoordinatorHost string) error {
	// Archive log directory on coordinator
	logDir, err := utils.GetLogDir()
	if err != nil {
		return err
	}

	log.Printf("archiving log directory %q to %q", logDir, logArchiveDir)
	if err = utils.Move(logDir, logArchiveDir); err != nil {
		return err
	}

	// Archive log directory on segments
	return ArchiveSegmentLogDirectories(agentConns, targetCoordinatorHost, logArchiveDir)

}

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
