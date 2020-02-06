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
	case "old-bindir":
		s.Source.BinDir = in.Value
	case "new-bindir":
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
	case "old-bindir":
		resp.Value = s.Source.BinDir
	case "new-bindir":
		resp.Value = s.Target.BinDir
	case "new-datadir":
		resp.Value = s.Target.MasterDataDir()
	default:
		return nil, status.Errorf(codes.NotFound, "%s is not a valid configuration key", in.Name)
	}

	return resp, nil
}
