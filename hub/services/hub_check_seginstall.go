package services

import (
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"context"
	"errors"
)

// grpc generated function signature requires ctx and in params.
// nolint: unparam
func (h *Hub) CheckSeginstall(ctx context.Context, in *pb.CheckSeginstallRequest) (*pb.CheckSeginstallReply, error) {
	gplog.Info("starting CheckSeginstall()")

	clusterHostnames, err := h.configreader.GetHostnames()
	if err != nil || len(clusterHostnames) == 0 {
		return &pb.CheckSeginstallReply{}, errors.New("no cluster config found, did you forget to run gpupgrade check config?")
	}

	go h.remoteExecutor.VerifySoftware(clusterHostnames)

	return &pb.CheckSeginstallReply{}, nil
}
