package services

import (
	"context"
	"errors"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (s *AgentServer) CheckConversionStatus(ctx context.Context, in *idl.CheckConversionStatusRequest) (*idl.CheckConversionStatusReply, error) {
	if len(in.GetSegments()) == 0 {
		return nil, errors.New("no segment information was passed to the agent")
	}

	var statuses []*idl.PrimaryStatus
	for _, segment := range in.GetSegments() {
		status := upgradestatus.SegmentConversionStatus(
			utils.SegmentPGUpgradeDirectory(s.conf.StateDir, int(segment.GetContent())),
			segment.GetDataDir(),
			s.executor,
		)

		statuses = append(statuses, &idl.PrimaryStatus{
			Status:   status,
			Dbid:     segment.GetDbid(),
			Content:  segment.GetContent(),
			Hostname: in.GetHostname(),
		})
	}

	return &idl.CheckConversionStatusReply{
		Statuses: statuses,
	}, nil
}
