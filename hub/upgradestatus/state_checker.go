package upgradestatus

import (
	"path/filepath"

	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

type StateCheck struct {
	path string
	step pb.UpgradeSteps
}

func NewStateCheck(path string, step pb.UpgradeSteps) StateCheck {
	return StateCheck{
		path: path,
		step: step,
	}
}

// GetStatus returns the UpgradeStepStatus corresponding to the StateCheck's
// step. Conceptually, this is one of (PENDING, RUNNING, COMPLETE, FAILED). This
// method will never return an error; instead, it will log any internal failures
// and return a PENDING status (because we currently expect that a re-run of the
// affected step should clear the issue).
//
// XXX That last assumption is unlikely to hold for the more complicated steps.
func (c StateCheck) GetStatus() *pb.UpgradeStepStatus {
	_, err := utils.System.Stat(c.path)
	if err != nil {
		// It's okay if the state directory doesn't exist; that just means we
		// haven't run the step yet.
		return c.newStatus(pb.StepStatus_PENDING)
	}

	files, err := utils.System.FilePathGlob(filepath.Join(c.path, "*"))
	if err != nil {
		// Log the error and keep the status PENDING.
		gplog.Error("Couldn't search status directory %s: %s", c.path, err.Error())
	}

	// FIXME: there's a race here: we delete the status file and then recreate
	// it in the ChecklistManager, which means we can go from RUNNING to PENDING
	// to COMPLETE/FAILED.
	if len(files) > 1 {
		gplog.Error("Status directory %s has more than one file", c.path)
		return c.newStatus(pb.StepStatus_PENDING)
	} else if len(files) == 1 {
		switch files[0] {
		case filepath.Join(c.path, "failed"):
			return c.newStatus(pb.StepStatus_FAILED)
		case filepath.Join(c.path, "completed"):
			return c.newStatus(pb.StepStatus_COMPLETE)
		case filepath.Join(c.path, "in.progress"):
			return c.newStatus(pb.StepStatus_RUNNING)
		}
	}
	return c.newStatus(pb.StepStatus_PENDING)
}

// newStatus builds a pb.UpgradeStepStatus using the current step.
func (c StateCheck) newStatus(status pb.StepStatus) *pb.UpgradeStepStatus {
	return &pb.UpgradeStepStatus{Step: c.step, Status: status}
}
