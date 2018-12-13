package services

import (
	"fmt"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/net/context"
	"github.com/pkg/errors"
	"github.com/greenplum-db/gpupgrade/utils/log"
)

func (h *Hub) UpgradeValidateStartCluster(ctx context.Context, in *idl.UpgradeValidateStartClusterRequest) (*idl.UpgradeValidateStartClusterReply, error) {
	gplog.Info("starting %s", upgradestatus.VALIDATE_START_CLUSTER)

	go func() {
		defer log.WritePanics()

		if err := h.startNewCluster(); err != nil {
			gplog.Error(err.Error())
		}
	}()

	return &idl.UpgradeValidateStartClusterReply{}, nil
}

func (h *Hub) startNewCluster() error {
	step := h.checklist.GetStepWriter(upgradestatus.VALIDATE_START_CLUSTER)
	err := step.ResetStateDir()
	if err != nil {
		return errors.Wrap(err, "failed to reset the state dir for validate-start-cluster")
	}

	err = step.MarkInProgress()
	if err != nil {
		return errors.Wrap(err, "failed to record in-progress for validate-start-cluster")
	}

	startCmd := fmt.Sprintf("source %s/../greenplum_path.sh; %s/gpstart -a -d %s", h.target.BinDir, h.target.BinDir, h.target.MasterDataDir())
	_, err = h.target.ExecuteLocalCommand(startCmd)
	if err != nil {
		return errors.Wrap(err, "failed to start new cluster")
	}

	return nil
}
