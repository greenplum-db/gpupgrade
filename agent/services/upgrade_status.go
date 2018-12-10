package services

import (
	"context"

	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

func (s *AgentServer) CheckUpgradeStatus(ctx context.Context, in *idl.CheckUpgradeStatusRequest) (*idl.CheckUpgradeStatusReply, error) {
	output, err := s.executor.ExecuteLocalCommand("ps auxx | grep pg_upgrade")
	if err != nil {
		gplog.Error(err.Error())
		return nil, err
	}
	return &idl.CheckUpgradeStatusReply{ProcessList: string(output)}, nil
}
