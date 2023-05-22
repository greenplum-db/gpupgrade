// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func ArchiveLogDirectories(logDir string, logArchiveDir string, agentConns []*idl.Connection, targetCoordinatorHost string) error {
	// Archive log directory on coordinator
	log.Printf("archiving log directory %q to %q", logDir, logArchiveDir)
	err := utils.Move(logDir, logArchiveDir)
	if err != nil {
		return err
	}

	// Archive log directory on segments
	return ArchiveSegmentLogDirectories(agentConns, targetCoordinatorHost, logArchiveDir)

}

func ArchiveSegmentLogDirectories(agentConns []*idl.Connection, excludeHostname, logArchiveDir string) error {
	request := func(conn *idl.Connection) error {
		if conn.Hostname == excludeHostname {
			return nil
		}

		req := &idl.ArchiveLogDirectoryRequest{LogArchiveDir: logArchiveDir}
		_, err := conn.AgentClient.ArchiveLogDirectory(context.Background(), req)
		return err
	}

	return ExecuteRPC(agentConns, request)
}

// GetLogArchiveDir returns the name of the file to be used to store logs
// from this run of gpupgrade during a revert.
func GetLogArchiveDir(logDir string, upgradeID string, t time.Time) string {
	archiveName := fmt.Sprintf("gpupgrade-%s-%s", upgradeID, t.Format("20060102T150409"))
	return filepath.Join(filepath.Dir(logDir), archiveName)
}
