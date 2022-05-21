// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"log"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

var RenameDirectories = upgrade.RenameDirectories

func (s *Server) RenameDirectories(ctx context.Context, in *idl.RenameDirectoriesRequest) (*idl.RenameDirectoriesReply, error) {
	log.Printf("starting %s", idl.Substep_update_data_directories)

	var mErr error
	for _, dir := range in.GetDirs() {
		err := RenameDirectories(dir.GetSource(), dir.GetTarget())
		if err != nil {
			mErr = errorlist.Append(mErr, err)
		}
	}

	return &idl.RenameDirectoriesReply{}, mErr
}
