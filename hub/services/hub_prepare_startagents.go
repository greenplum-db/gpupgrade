package services

import (
	pb "github.com/greenplum-db/gpupgrade/idl"
	"context"
	"errors"
)

// grpc generated function signature requires ctx and in params.
// nolint: unparam
func (h *Hub) PrepareStartAgents(ctx context.Context,
	in *pb.PrepareStartAgentsRequest) (*pb.PrepareStartAgentsReply, error) {

	clusterHostnames, err := h.configreader.GetHostnames()
	if err != nil || len(clusterHostnames) == 0 {
		return &pb.PrepareStartAgentsReply{}, errors.New("no cluster config found, did you forget to run gpupgrade check config?")
	}

	go h.remoteExecutor.Start(clusterHostnames)

	return &pb.PrepareStartAgentsReply{}, nil
}
