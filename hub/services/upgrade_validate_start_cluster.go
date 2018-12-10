package services

import (
	"fmt"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/net/context"
)

func (h *Hub) UpgradeValidateStartCluster(ctx context.Context, in *idl.UpgradeValidateStartClusterRequest) (*idl.UpgradeValidateStartClusterReply, error) {
	gplog.Info("Started processing validate-start-cluster request")

	go h.startNewCluster()

	return &idl.UpgradeValidateStartClusterReply{}, nil
}

func (h *Hub) startNewCluster() {
	gplog.Debug(h.conf.StateDir)
	step := h.checklist.GetStepWriter(upgradestatus.VALIDATE_START_CLUSTER)
	err := step.ResetStateDir()
	if err != nil {
		gplog.Error("failed to reset the state dir for validate-start-cluster")

		return
	}

	err = step.MarkInProgress()
	if err != nil {
		gplog.Error("failed to record in-progress for validate-start-cluster")

		return
	}

	targetBinDir := h.target.BinDir
	targetDataDir := h.target.MasterDataDir()
	_, err = h.target.ExecuteLocalCommand(fmt.Sprintf("source %s/../greenplum_path.sh; %s/gpstart -a -d %s", targetBinDir, targetBinDir, targetDataDir))
	if err != nil {
		gplog.Error(err.Error())
		cmErr := step.MarkFailed()
		if cmErr != nil {
			gplog.Error("failed to record failed for validate-start-cluster")
		}

		return
	}

	err = step.MarkComplete()
	if err != nil {
		gplog.Error("failed to record completed for validate-start-cluster")
		return
	}

	return
}
