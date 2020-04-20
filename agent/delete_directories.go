// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

var deleteDirectories = upgrade.DeleteDirectories

func (s *Server) DeleteStateDirectory(ctx context.Context, in *idl.DeleteStateDirectoryRequest) (*idl.DeleteStateDirectoryReply, error) {
	gplog.Info("got a request to delete the state directory from the hub")

	err := deleteDirectories([]string{s.conf.StateDir}, upgrade.StateDirectoryFiles)
	return &idl.DeleteStateDirectoryReply{}, err
}

func (s *Server) DeleteDataDirectories(ctx context.Context, in *idl.DeleteDataDirectoriesRequest) (*idl.DeleteDataDirectoriesReply, error) {
	gplog.Info("got a request to delete data directories from the hub")

	err := deleteDirectories(in.Datadirs, upgrade.PostgresFiles)
	return &idl.DeleteDataDirectoriesReply{}, err
}
