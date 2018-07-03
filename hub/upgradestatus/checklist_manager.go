package upgradestatus

import (
	"os"
	"path"
	"path/filepath"

	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

const (
	CONFIG                 = "check-config"
	SEGINSTALL             = "check-seginstall"
	START_AGENTS           = "start-agents"
	INIT_CLUSTER           = "init-cluster"
	SHUTDOWN_CLUSTERS      = "shutdown-clusters"
	CONVERT_MASTER         = "convert-master"
	SHARE_OIDS             = "share-oids"
	CONVERT_PRIMARIES      = "convert-primaries"
	VALIDATE_START_CLUSTER = "validate-start-cluster"
	RECONFIGURE_PORTS      = "reconfigure-ports"
)

const (
	fs_inprogress = "in.progress"
	fs_failed     = "failed"
	fs_completed  = "completed"
)

type Checklist interface {
	LoadSteps(steps []Step) // XXX Feels like this is an implementation detail.
	AllSteps() []StateReader
	GetStepReader(step string) StateReader
	GetStepWriter(step string) StateWriter
}

type StateReader interface {
	Name() string
	Code() pb.UpgradeSteps
	Status() pb.StepStatus
}

type StateWriter interface {
	MarkInProgress() error
	ResetStateDir() error
	MarkFailed() error
	MarkComplete() error
}

type ChecklistManager struct {
	pathToStateDir string
	steps          []StateReader
	stepmap        map[string]StateReader // maps step name to StateReader implementation
}

type Step struct {
	Name_   string
	Code_   pb.UpgradeSteps
	Status_ func(r StateReader) pb.StepStatus
}

func (s Step) Name() string {
	return s.Name_
}

func (s Step) Code() pb.UpgradeSteps {
	return s.Code_
}

func (s Step) Status() pb.StepStatus {
	return s.Status_(s)
}

func NewChecklistManager(stateDirPath string) *ChecklistManager {
	return &ChecklistManager{
		pathToStateDir: stateDirPath,
		stepmap:        map[string]StateReader{},
	}
}

func (c *ChecklistManager) LoadSteps(steps []Step) {
	c.steps = make([]StateReader, len(steps))
	c.stepmap = map[string]StateReader{}
	for i, step := range steps {
		c.steps[i] = step
		c.stepmap[step.Name_] = step
	}
}

func (c *ChecklistManager) GetStepReader(step string) StateReader {
	return c.stepmap[step]
}

func (c *ChecklistManager) AllSteps() []StateReader {
	return c.steps
}

func (c *ChecklistManager) GetStepWriter(step string) StateWriter {
	stepdir := filepath.Join(c.pathToStateDir, step)
	return StepWriter{stepdir: stepdir}
}

type StepWriter struct {
	stepdir string // path to step-specific state directory
}

// FIXME: none of these operations are atomic on the FS; just move the progress
// file from name to name instead
func (sw StepWriter) MarkFailed() error {
	err := utils.System.Remove(filepath.Join(sw.stepdir, fs_inprogress))
	if err != nil {
		return err
	}

	_, err = utils.System.OpenFile(path.Join(sw.stepdir, fs_failed), os.O_CREATE, 0700)
	if err != nil {
		return err
	}

	return nil
}

func (sw StepWriter) MarkComplete() error {
	err := utils.System.Remove(filepath.Join(sw.stepdir, fs_inprogress))
	if err != nil {
		return err
	}

	_, err = utils.System.OpenFile(path.Join(sw.stepdir, fs_completed), os.O_CREATE, 0700)
	if err != nil {
		return err
	}

	return nil
}

func (sw StepWriter) MarkInProgress() error {
	_, err := utils.System.OpenFile(path.Join(sw.stepdir, fs_inprogress), os.O_CREATE, 0700)
	if err != nil {
		return err
	}

	return nil
}

func (sw StepWriter) ResetStateDir() error {
	err := utils.System.RemoveAll(sw.stepdir)
	if err != nil {
		return err
	}

	err = utils.System.MkdirAll(sw.stepdir, 0700)
	if err != nil {
		return err
	}

	return nil
}
