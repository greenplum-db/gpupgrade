package upgradestatus

import (
	"path/filepath"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus/file"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

type StateCheck struct {
	Path string
	Step idl.Substep
}

// GetStatus returns the UpgradeStepStatus corresponding to the StateCheck's
// step. Conceptually, this is one of (PENDING, RUNNING, COMPLETE, FAILED). This
// method will never return an error; instead, it will log any internal failures
// and return a PENDING status (because we currently expect that a re-run of the
// affected step should clear the issue).
//
// XXX That last assumption is unlikely to hold for the more complicated steps.
func (c StateCheck) GetStatus() idl.Status {
	_, err := utils.System.Stat(c.Path)
	if err != nil {
		// It's okay if the state directory doesn't exist; that just means we
		// haven't run the step yet.
		return idl.Status_PENDING
	}

	files, err := utils.System.FilePathGlob(filepath.Join(c.Path, "*"))
	if err != nil {
		// Log the error and keep the status PENDING.
		gplog.Error("Couldn't search status directory %s: %s", c.Path, err.Error())
	}

	// FIXME: there's a race here: we delete the status file and then recreate
	// it in the ChecklistManager, which means we can go from RUNNING to PENDING
	// to COMPLETE/FAILED.
	if len(files) > 1 {
		gplog.Error("Status directory %s has more than one file", c.Path)
		return idl.Status_PENDING
	} else if len(files) == 1 {
		switch files[0] {
		case filepath.Join(c.Path, file.Failed):
			return idl.Status_FAILED
		case filepath.Join(c.Path, file.Complete):
			return idl.Status_COMPLETE
		case filepath.Join(c.Path, file.InProgress):
			return idl.Status_RUNNING
		}
	}
	return idl.Status_PENDING
}
