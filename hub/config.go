package hub

import (
	"github.com/greenplum-db/gpupgrade/idl"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/net/context"
)

const ConfigFileName = "config.json"

func (h *Server) SetConfig(ctx context.Context, in *idl.SetConfigRequest) (*idl.SetConfigReply, error) {
	switch in.Name {
	case "old-bindir":
		h.Source.BinDir = in.Value
	case "new-bindir":
		h.Target.BinDir = in.Value
	default:
		return nil, status.Errorf(codes.NotFound, "%s is not a valid configuration key", in.Name)
	}

	if err := h.SaveConfig(); err != nil {
		return &idl.SetConfigReply{}, err
	}

	gplog.Info("Successfully set %s to %s", in.Name, in.Value)
	return &idl.SetConfigReply{}, nil
}

func (h *Server) GetConfig(ctx context.Context, in *idl.GetConfigRequest) (*idl.GetConfigReply, error) {
	resp := &idl.GetConfigReply{}

	switch in.Name {
	case "old-bindir":
		resp.Value = h.Source.BinDir
	case "new-bindir":
		resp.Value = h.Target.BinDir
	case "new-datadir":
		resp.Value = h.Target.MasterDataDir()
	default:
		return nil, status.Errorf(codes.NotFound, "%s is not a valid configuration key", in.Name)
	}

	return resp, nil
}
