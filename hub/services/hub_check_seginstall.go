package services

import (
	"context"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	pb "github.com/greenplum-db/gpupgrade/idl"
)

// grpc generated function signature requires ctx and in params.
// nolint: unparam
func (h *Hub) CheckSeginstall(ctx context.Context, in *pb.CheckSeginstallRequest) (*pb.CheckSeginstallReply, error) {
	gplog.Info("starting CheckSeginstall()")

	go h.remoteExecutor.VerifySoftware(h.clusterPair.GetHostnames())

	return &pb.CheckSeginstallReply{}, nil
}
