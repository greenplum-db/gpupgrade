package services

import (
	"context"

	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

func (s *AgentServer) PingAgents(ctx context.Context, in *idl.PingAgentsRequest) (*idl.PingAgentsReply, error) {
	gplog.Info("Successfully pinged agent")
	return &idl.PingAgentsReply{}, nil
}
