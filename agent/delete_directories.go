// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

var deleteDirectories = upgrade.DeleteDirectories

func (s *Server) DeleteStateDirectory(ctx context.Context, in *idl.DeleteStateDirectoryRequest) (*idl.DeleteStateDirectoryReply, error) {
	gplog.Info("got a request to delete the state directory from the hub")

	hostname, err := utils.System.Hostname()
	if err != nil {
		return &idl.DeleteStateDirectoryReply{}, err
	}

	err = deleteDirectories([]string{s.conf.StateDir}, upgrade.StateDirectoryFiles, hostname, step.DevNullStream)
	return &idl.DeleteStateDirectoryReply{}, err
}

func (s *Server) DeleteDataDirectories(ctx context.Context, in *idl.DeleteDataDirectoriesRequest) (*idl.DeleteDataDirectoriesReply, error) {
	gplog.Info("got a request to delete data directories from the hub")

	hostname, err := utils.System.Hostname()
	if err != nil {
		return &idl.DeleteDataDirectoriesReply{}, err
	}

	err = deleteDirectories(in.Datadirs, upgrade.PostgresFiles, hostname, step.DevNullStream)
	return &idl.DeleteDataDirectoriesReply{}, err
}
