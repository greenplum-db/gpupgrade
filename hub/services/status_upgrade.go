package services

import (
	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/net/context"
)

func (h *Hub) StatusUpgrade(ctx context.Context, in *idl.StatusUpgradeRequest) (*idl.StatusUpgradeReply, error) {
	gplog.Info("starting StatusUpgrade")

	steps := h.checklist.AllSteps()
	statuses := make([]*idl.UpgradeStepStatus, len(steps))

	for i, step := range steps {
		gplog.Info("Checking %s...", step.Name())
		statuses[i] = &idl.UpgradeStepStatus{Step: step.Code(), Status: step.Status()}
	}

	return &idl.StatusUpgradeReply{
		ListOfUpgradeStepStatuses: statuses,
	}, nil
}
