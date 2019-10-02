package stream_step_status

import (
	"github.com/greenplum-db/gpupgrade/idl"
)

func StreamStepStatus(stream idl.CliToHub_InitializeServer, upgradeStep idl.UpgradeSteps, stepStatus idl.StepStatus) error {
	err := stream.Send(&idl.UpgradeStream{
		Status: &idl.UpgradeStepStatus{
			Step:   upgradeStep,
			Status: stepStatus,
		},
		Type: idl.UpgradeStream_STEP_STATUS,
	})

	return err
}
