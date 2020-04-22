// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

var ArchiveSource = upgrade.ArchiveSource

func (s *Server) RenameDirectories(ctx context.Context, in *idl.RenameDirectoriesRequest) (*idl.RenameDirectoriesReply, error) {
	gplog.Info("agent received request to rename segment data directories")

	var mErr *multierror.Error
	for _, dir := range in.GetDirs() {
		err := ArchiveSource(dir.Source, dir.Archive, dir.Target, dir.RenameTarget)
		if err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	return &idl.RenameDirectoriesReply{}, mErr.ErrorOrNil()
}
