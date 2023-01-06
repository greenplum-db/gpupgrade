// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"strconv"

	"github.com/greenplum-db/gpupgrade/idl"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) GetConfig(ctx context.Context, in *idl.GetConfigRequest) (*idl.GetConfigReply, error) {
	resp := &idl.GetConfigReply{}

	switch in.Name {
	case "id":
		resp.Value = s.UpgradeID.String()
	case "source-gphome":
		if s.Source != nil {
			resp.Value = s.Source.GPHome
		}
	case "target-gphome":
		resp.Value = s.Intermediate.GPHome
	case "target-datadir":
		if s.Target != nil {
			resp.Value = s.Intermediate.CoordinatorDataDir()
		}
	case "target-port":
		if s.Intermediate.CoordinatorPort() != 0 {
			resp.Value = strconv.Itoa(s.Intermediate.CoordinatorPort())
		}
	default:
		return nil, status.Errorf(codes.NotFound, "%q is not a valid configuration key", in.Name)
	}

	return resp, nil
}
