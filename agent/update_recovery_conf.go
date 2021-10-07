// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func (s *Server) UpdateRecoveryConf(ctx context.Context, in *idl.UpdateRecoveryConfRequest) (*idl.UpdateRecoveryConfReply, error) {
	gplog.Info("agent received request to update recovery.conf")

	var errs error
	for _, opt := range in.GetOptions() {
		err := hub.UpdateRecoveryConf(opt.GetPath(), int(opt.GetCurrentValue()), int(opt.GetUpdatedValue()))
		if err != nil {
			errs = errorlist.Append(errs, err)
		}
	}

	return &idl.UpdateRecoveryConfReply{}, errs
}
