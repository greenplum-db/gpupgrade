// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"github.com/greenplum-db/gpupgrade/idl"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/net/context"
)

const ConfigFileName = "config.json"

func (s *Server) SetConfig(ctx context.Context, in *idl.SetConfigRequest) (*idl.SetConfigReply, error) {
	switch in.Name {
	case "source-bindir":
		s.Source.BinDir = in.Value
	case "target-bindir":
		s.Target.BinDir = in.Value
	default:
		return nil, status.Errorf(codes.NotFound, "%s is not a valid configuration key", in.Name)
	}

	if err := s.SaveConfig(); err != nil {
		return &idl.SetConfigReply{}, err
	}

	gplog.Info("Successfully set %s to %s", in.Name, in.Value)
	return &idl.SetConfigReply{}, nil
}

func (s *Server) GetConfig(ctx context.Context, in *idl.GetConfigRequest) (*idl.GetConfigReply, error) {
	resp := &idl.GetConfigReply{}

	switch in.Name {
	case "id":
		resp.Value = s.UpgradeID.String()
	case "source-bindir":
		resp.Value = s.Source.BinDir
	case "target-bindir":
		resp.Value = s.Target.BinDir
	case "target-datadir":
		resp.Value = s.Target.MasterDataDir()
	default:
		return nil, status.Errorf(codes.NotFound, "%s is not a valid configuration key", in.Name)
	}

	return resp, nil
}
