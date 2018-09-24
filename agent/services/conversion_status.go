package services

import (
	"context"
	"errors"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (s *AgentServer) CheckConversionStatus(ctx context.Context, in *pb.CheckConversionStatusRequest) (*pb.CheckConversionStatusReply, error) {
	if len(in.GetSegments()) == 0 {
		return nil, errors.New("no segment information was passed to the agent")
	}

	var statuses []*pb.PrimaryStatus
	for _, segment := range in.GetSegments() {
		status := upgradestatus.SegmentConversionStatus(
			utils.SegmentPGUpgradeDirectory(s.conf.StateDir, int(segment.GetContent())),
			segment.GetDataDir(),
			s.executor,
		)

		statuses = append(statuses, &pb.PrimaryStatus{
			Status:   status,
			Dbid:     segment.GetDbid(),
			Content:  segment.GetContent(),
			Hostname: in.GetHostname(),
		})
	}

	return &pb.CheckConversionStatusReply{
		Statuses: statuses,
	}, nil
}
