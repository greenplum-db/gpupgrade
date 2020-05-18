// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (s *Server) ArchiveLogDirectory(ctx context.Context, in *idl.ArchiveLogDirectoryRequest) (*idl.ArchiveLogDirectoryReply, error) {
	gplog.Info("agent starting %s", idl.Substep_ARCHIVE_LOG_DIRECTORIES)

	logdir, err := utils.GetLogDir()
	if err != nil {
		return &idl.ArchiveLogDirectoryReply{}, err
	}
	if err = utils.System.Rename(logdir, in.GetNewDir()); err != nil {
		if utils.System.IsNotExist(err) {
			gplog.Debug("log directory %s not archived, possibly due to multi-host environment. %+v", logdir, err)
			err = nil
		}
	}
	return &idl.ArchiveLogDirectoryReply{}, err
}
