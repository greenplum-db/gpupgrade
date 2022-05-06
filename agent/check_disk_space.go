// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils/disk"
)

func (s *Server) CheckDiskSpace(ctx context.Context, in *idl.CheckSegmentDiskSpaceRequest) (*idl.CheckDiskSpaceReply, error) {
	gplog.Info("agent received request to %s", idl.Substep_CHECK_DISK_SPACE)

	usage, err := disk.CheckUsage(step.DevNullStream, disk.Local, in.GetDiskFreeRatio(), in.GetDirs()...)
	if err != nil {
		return nil, err
	}

	return &idl.CheckDiskSpaceReply{Usage: usage}, nil
}
