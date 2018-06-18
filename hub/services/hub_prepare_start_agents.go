package services

import (
	"context"

	pb "github.com/greenplum-db/gpupgrade/idl"
)

// grpc generated function signature requires ctx and in params.
// nolint: unparam
func (h *Hub) PrepareStartAgents(ctx context.Context,
	in *pb.PrepareStartAgentsRequest) (*pb.PrepareStartAgentsReply, error) {

	go h.remoteExecutor.Start(h.clusterPair.GetHostnames())

	return &pb.PrepareStartAgentsReply{}, nil
}
