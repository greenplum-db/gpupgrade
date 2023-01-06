// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

func DeleteBackupDirectories(streams step.OutStreams, agentConns []*idl.Connection, backupDir string) error {
	// delete on coordinator
	err := upgrade.DeleteDirectories([]string{backupDir}, []string{}, streams)
	if err != nil {
		return err
	}

	// delete on segments
	request := func(conn *idl.Connection) error {
		req := &idl.DeleteBackupDirectoryRequest{BackupDir: backupDir}
		_, err := conn.AgentClient.DeleteBackupDirectory(context.Background(), req)
		return err
	}

	return ExecuteRPC(agentConns, request)
}
