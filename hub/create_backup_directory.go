// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"fmt"
	"log"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

func CreateBackupDirectories(streams step.OutStreams, agentConns []*idl.Connection, backupDir string) error {
	_, err := fmt.Fprintf(streams.Stdout(), "creating backup directory %q on all hosts\n", backupDir)
	if err != nil {
		return err
	}

	// create on coordinator
	err = CreateBackupDirectory(backupDir)
	if err != nil {
		return err
	}

	// create on segments
	request := func(conn *idl.Connection) error {
		req := &idl.CreateBackupDirectoryRequest{BackupDir: backupDir}
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
