// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
)

func (s *Server) UpdateRecoveryConf(ctx context.Context, req *idl.UpdateRecoveryConfRequest) (*idl.UpdateRecoveryConfReply, error) {
	gplog.Info("agent received request to update recovery.conf")

	err := hub.UpdateRecoveryConf(req.GetOptions())
	if err != nil {
		return &idl.UpdateRecoveryConfReply{}, err
	}

	return &idl.UpdateRecoveryConfReply{}, nil
}
