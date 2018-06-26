package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
)

func (s *AgentServer) CheckConversionStatus(ctx context.Context, in *pb.CheckConversionStatusRequest) (*pb.CheckConversionStatusReply, error) {
	if len(in.GetSegments()) == 0 {
		return nil, errors.New("no segment information was passed to the agent")
	}
	format := "%s - DBID %d - CONTENT ID %d - %s - %s"

	var replies []string
	var master string
	for _, segment := range in.GetSegments() {
		conversionStatus := upgradestatus.NewPGUpgradeStatusChecker(
			upgradestatus.PRIMARY,
			filepath.Join(s.conf.StateDir, "pg_upgrade", fmt.Sprintf("seg-%d", segment.GetContent())),
			segment.GetDataDir(),
			s.commandExecer,
		)

		status := conversionStatus.GetStatus()

		if segment.GetDbid() == 1 && segment.GetContent() == -1 {
			master = fmt.Sprintf(format, status.Status.String(), segment.GetDbid(), segment.GetContent(), "MASTER", in.GetHostname())
		} else {
			replies = append(replies, fmt.Sprintf(format, status.Status.String(), segment.GetDbid(), segment.GetContent(), "PRIMARY", in.GetHostname()))
		}
	}

	replies = append([]string{master}, replies...)

	return &pb.CheckConversionStatusReply{
		Statuses: replies,
	}, nil
}
