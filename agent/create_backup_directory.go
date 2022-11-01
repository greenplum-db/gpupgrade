// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
)

func (s *Server) CreateBackupDirectory(ctx context.Context, req *idl.CreateBackupDirectoryRequest) (*idl.CreateBackupDirectoryReply, error) {
	log.Printf("starting %s", idl.Substep_create_backupdir)

	hostname, err := os.Hostname()
	if err != nil {
		return &idl.CreateBackupDirectoryReply{}, err
	}

	err = hub.CreateBackupDirectory(req.GetBackupDir())
	if err != nil {
		return &idl.CreateBackupDirectoryReply{}, fmt.Errorf("on host %q: %w", hostname, err)
	}

	return &idl.CreateBackupDirectoryReply{}, nil
}
