package services

import (
	pb "github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/net/context"
)

func (h *Hub) StatusUpgrade(ctx context.Context, in *pb.StatusUpgradeRequest) (*pb.StatusUpgradeReply, error) {
	gplog.Info("starting StatusUpgrade")

	steps := h.checklist.AllSteps()
	statuses := make([]*pb.UpgradeStepStatus, len(steps))

	for i, step := range steps {
		gplog.Info("Checking %s...", step.Name())
		statuses[i] = &pb.UpgradeStepStatus{Step: step.Code(), Status: step.Status()}
	}

	return &pb.StatusUpgradeReply{
		ListOfUpgradeStepStatuses: statuses,
	}, nil
}
