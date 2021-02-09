// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func (s *Server) RestorePrimariesPgControl(ctx context.Context, in *idl.RestorePgControlRequest) (*idl.RestorePgControlReply, error) {
	var mErr error

	for _, dir := range in.Datadirs {
		err := upgrade.RestorePgControl(dir, step.DevNullStream)
		if err != nil {
			mErr = errorlist.Append(mErr, err)
		}
	}

	return &idl.RestorePgControlReply{}, mErr
}
