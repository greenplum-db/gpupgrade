package services

import (
	pb "github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/net/context"
)

func (h *Hub) Ping(ctx context.Context, in *pb.PingRequest) (*pb.PingReply, error) {
	gplog.Info("starting Ping")
	return &pb.PingReply{}, nil
}
