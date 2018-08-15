package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (s *AgentServer) CheckConversionStatus(ctx context.Context, in *pb.CheckConversionStatusRequest) (*pb.CheckConversionStatusReply, error) {
	if len(in.GetSegments()) == 0 {
		return nil, errors.New("no segment information was passed to the agent")
	}
	format := "%s - DBID %d - CONTENT ID %d - PRIMARY - %s"

	var replies []string
	for _, segment := range in.GetSegments() {
		status := upgradestatus.SegmentConversionStatus(
			utils.SegmentPGUpgradeDirectory(s.conf.StateDir, int(segment.GetContent())),
			segment.GetDataDir(),
			s.executor,
		)

		// FIXME: we have status codes; why convert to strings?
		replies = append(replies, fmt.Sprintf(format, status.String(), segment.GetDbid(), segment.GetContent(), in.GetHostname()))
	}

	return &pb.CheckConversionStatusReply{
		Statuses: replies,
	}, nil
}
