// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
)

func (s *Server) UpdatePostgresqlConf(ctx context.Context, req *idl.UpdatePostgresqlConfRequest) (*idl.UpdatePostgresqlConfReply, error) {
	gplog.Info("agent received request to update postgresql.conf")

	err := hub.UpdatePostgresqlConf(req.GetOptions())
	if err != nil {
		return &idl.UpdatePostgresqlConfReply{}, err
	}

	return &idl.UpdatePostgresqlConfReply{}, nil
}
