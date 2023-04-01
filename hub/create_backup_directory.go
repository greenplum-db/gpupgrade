// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"fmt"
	"log"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/config/backupdir"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

func CreateBackupDirectories(streams step.OutStreams, agentConns []*idl.Connection, backupDirs backupdir.BackupDirs) error {
	_, err := fmt.Fprintf(streams.Stdout(), "creating backup directory on all hosts\n")
	if err != nil {
		return err
	}

	err = CreateBackupDirectory(backupDirs.CoordinatorBackupDir)
	if err != nil {
		return err
	}

	request := func(conn *idl.Connection) error {
		if _, ok := backupDirs.AgentHostsToBackupDir[conn.Hostname]; !ok {
			return nil
		}

		req := &idl.CreateBackupDirectoryRequest{BackupDir: backupDirs.AgentHostsToBackupDir[conn.Hostname]}
		_, err = conn.AgentClient.CreateBackupDirectory(context.Background(), req)
		return err
	}

	return ExecuteRPC(agentConns, request)
}

func CreateBackupDirectory(backupDir string) error {
	log.Printf("creating backup directory %q", backupDir)
	err := utils.System.MkdirAll(backupDir, 0700)
	if err != nil {
		return xerrors.Errorf("create backup directory %q: %w", backupDir, err)
	}

	return nil
}
