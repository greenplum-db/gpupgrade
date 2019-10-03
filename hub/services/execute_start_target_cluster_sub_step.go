package services

import (
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/pkg/errors"
)

func (h *Hub) ExecuteStartTargetClusterSubStep(stream idl.CliToHub_ExecuteServer) error {
	gplog.Info("starting %s", upgradestatus.VALIDATE_START_CLUSTER)

	step, err := h.InitializeStep(upgradestatus.VALIDATE_START_CLUSTER)
	if err != nil {
		gplog.Error(err.Error())
		return err
	}

	_ = stream.Send(&idl.ExecuteMessage{
		Contents: &idl.ExecuteMessage_Status{&idl.UpgradeStepStatus{
			Step:   idl.UpgradeSteps_VALIDATE_START_CLUSTER,
			Status: idl.StepStatus_RUNNING,
		}},
	})

	err = h.startNewCluster()
	if err != nil {
		gplog.Error(err.Error())
		step.MarkFailed()

		_ = stream.Send(&idl.ExecuteMessage{
			Contents: &idl.ExecuteMessage_Status{&idl.UpgradeStepStatus{
				Step:   idl.UpgradeSteps_VALIDATE_START_CLUSTER,
				Status: idl.StepStatus_FAILED,
			}},
		})
	} else {
		step.MarkComplete()

		_ = stream.Send(&idl.ExecuteMessage{
			Contents: &idl.ExecuteMessage_Status{&idl.UpgradeStepStatus{
				Step:   idl.UpgradeSteps_VALIDATE_START_CLUSTER,
				Status: idl.StepStatus_COMPLETE,
			}},
		})
	}

	return nil
}

func (h *Hub) startNewCluster() error {
	startCmd := fmt.Sprintf("source %s/../greenplum_path.sh; %s/gpstart -a -d %s", h.target.BinDir, h.target.BinDir, h.target.MasterDataDir())
	_, err := h.target.ExecuteLocalCommand(startCmd)
	if err != nil {
		return errors.Wrap(err, "failed to start new cluster")
	}

	return nil
}
