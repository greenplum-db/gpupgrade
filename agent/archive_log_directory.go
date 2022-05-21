// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"log"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (s *Server) ArchiveLogDirectory(ctx context.Context, in *idl.ArchiveLogDirectoryRequest) (*idl.ArchiveLogDirectoryReply, error) {
	log.Printf("starting %s", idl.Substep_archive_log_directories)

	logdir, err := utils.GetLogDir()
	if err != nil {
		return &idl.ArchiveLogDirectoryReply{}, err
	}

	log.Printf("moving directory %q to %q", logdir, in.GetNewDir())
	err = utils.Move(logdir, in.GetNewDir())
	return &idl.ArchiveLogDirectoryReply{}, err
}
