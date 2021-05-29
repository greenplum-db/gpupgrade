// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils/disk"
)

func (s *Server) CheckDiskSpace(ctx context.Context, in *idl.CheckSegmentDiskSpaceRequest) (*idl.CheckDiskSpaceReply, error) {
	usage, err := disk.CheckUsage(step.DevNullStream, disk.Local, in.GetDiskFreeRatio(), in.GetDirs()...)
	if err != nil {
		return nil, err
	}

	return &idl.CheckDiskSpaceReply{Usage: usage}, nil
}
