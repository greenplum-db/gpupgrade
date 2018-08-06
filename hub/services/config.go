package services

import (
	pb "github.com/greenplum-db/gpupgrade/idl"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/net/context"
)

func (h *Hub) SetConfig(ctx context.Context, in *pb.SetConfigRequest) (*pb.SetConfigReply, error) {
	switch in.Name {
	case "old-bindir":
		h.source.BinDir = in.Value
	case "new-bindir":
		h.target.BinDir = in.Value
	default:
		return nil, status.Errorf(codes.NotFound, "%s is not a valid configuration key", in.Name)
	}

	// Persist.
	err := h.source.Commit()
	if err != nil {
		return &pb.SetConfigReply{}, err
	}
	err = h.target.Commit()
	if err != nil {
		return &pb.SetConfigReply{}, err
	}

	gplog.Info("Successfully set %s to %s", in.Name, in.Value)
	return &pb.SetConfigReply{}, nil
}

func (h *Hub) GetConfig(ctx context.Context, in *pb.GetConfigRequest) (*pb.GetConfigReply, error) {
	resp := &pb.GetConfigReply{}

	switch in.Name {
	case "old-bindir":
		resp.Value = h.source.BinDir
	case "new-bindir":
		resp.Value = h.target.BinDir
	default:
		return nil, status.Errorf(codes.NotFound, "%s is not a valid configuration key", in.Name)
	}

	return resp, nil
}
