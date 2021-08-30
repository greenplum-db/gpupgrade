// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

var RenameDirectories = upgrade.RenameDirectories

func (s *Server) RenameDirectories(ctx context.Context, in *idl.RenameDirectoriesRequest) (*idl.RenameDirectoriesReply, error) {
	gplog.Info("agent received request to rename segment data directories")

	var mErr error
	for _, dir := range in.GetDirs() {
		err := RenameDirectories(dir.GetSource(), dir.GetTarget(), dir.GetRenameDirectory())
		if err != nil {
			mErr = errorlist.Append(mErr, err)
		}
	}

	return &idl.RenameDirectoriesReply{}, mErr
}
