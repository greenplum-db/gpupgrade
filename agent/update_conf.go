// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
)

func (s *Server) UpdateConfiguration(ctx context.Context, req *idl.UpdateConfigurationRequest) (*idl.UpdateConfigurationReply, error) {
	gplog.Info("agent received request to update configuration file")

	err := hub.UpdateConfigurationFile(req.GetOptions())
	if err != nil {
		return &idl.UpdateConfigurationReply{}, err
	}

	return &idl.UpdateConfigurationReply{}, nil
}
