// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"log"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils/disk"
)

func (s *Server) CheckDiskSpace(ctx context.Context, in *idl.CheckSegmentDiskSpaceRequest) (*idl.CheckDiskSpaceReply, error) {
	log.Printf("starting %s", idl.Substep_check_disk_space)

	usage, err := disk.CheckUsage(step.DevNullStream, disk.Local, in.GetDiskFreeRatio(), in.GetDirs()...)
	if err != nil {
		return nil, err
	}

	return &idl.CheckDiskSpaceReply{Usages: usage}, nil
}
