package upgradestatus

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus/file"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

const (
	CONFIG                 = "check-config"
	START_AGENTS           = "start-agents"
	INIT_CLUSTER           = "init-cluster"
	SHUTDOWN_CLUSTERS      = "shutdown-clusters"
	CONVERT_MASTER         = "convert-master"
	COPY_MASTER            = "copy-master"
	CONVERT_PRIMARIES      = "convert-primaries"
	VALIDATE_START_CLUSTER = "validate-start-cluster"
	RECONFIGURE_PORTS      = "reconfigure-ports"
)

type Checklist interface {
	AllSteps() []StateReader
	GetStepReader(step string) StateReader
	GetStepWriter(step string) StateWriter
}

type StateReader interface {
	Name() string
	Code() idl.UpgradeSteps
	Status() idl.StepStatus
}

type StateWriter interface {
	MarkInProgress() error
	ResetStateDir() error
	MarkFailed() error
	MarkComplete() error
}

type ChecklistManager struct {
	stateDir string                 // the directory where writable step status is kept
	steps    []StateReader          // backing slice for AllSteps()
	stepmap  map[string]StateReader // maps step name to StateReader implementation
	readOnly map[string]bool        // value is true iff step was added via AddReadOnlyStep()
}

// A StatusFunc returns a StepStatus for a read-only step. It is passed the name
// of the step to facilitate sharing of step implementations.
type StatusFunc func(name string) idl.StepStatus

type step struct {
	name   string
	code   idl.UpgradeSteps
	status StatusFunc
}

func (s step) Name() string {
	return s.name
}

func (s step) Code() idl.UpgradeSteps {
	return s.code
}

func (s step) Status() idl.StepStatus {
	return s.status(s.name)
}

func NewChecklistManager(stateDirPath string) *ChecklistManager {
	return &ChecklistManager{
		stateDir: stateDirPath,
		stepmap:  map[string]StateReader{},
		readOnly: map[string]bool{},
	}
}

// AddWritableStep creates a step with a writable status that is backed by the
// filesystem. The given name must be filesystem-friendly, since it will be used
// in the backing path.
func (c *ChecklistManager) AddWritableStep(name string, code idl.UpgradeSteps) {
	statusFunc := func(name string) idl.StepStatus {
		checker := StateCheck{
			Path: filepath.Join(c.stateDir, name),
			Step: code,
		}
		return checker.GetStatus()
	}

	c.addStep(name, code, statusFunc)
}

// AddReadOnlyStep creates a step with a custom status retrieval mechanism, as
// determined by the given StatusFunc.
func (c *ChecklistManager) AddReadOnlyStep(name string, code idl.UpgradeSteps, status StatusFunc) {
	c.addStep(name, code, status)
	c.readOnly[name] = true
}

func (c *ChecklistManager) addStep(name string, code idl.UpgradeSteps, status StatusFunc) {
	s := step{name, code, status}

	// Since checklist setup isn't influenced by the user, it's always a
	// programmer error for a step to be added twice. Panic instead of making
	// all callers check for an error that should never happen.
	if _, ok := c.stepmap[name]; ok {
		panic(fmt.Sprintf(`step "%s" has already been added`, name))
	}

	c.steps = append(c.steps, s)
	c.stepmap[name] = s
}

func (c *ChecklistManager) GetStepReader(step string) StateReader {
	return c.stepmap[step]
}

func (c *ChecklistManager) AllSteps() []StateReader {
	return c.steps
}

func (c *ChecklistManager) GetStepWriter(step string) StateWriter {
	if c.readOnly[step] {
		// This is always a programmer error: we shouldn't ever write to a
		// read-only step. Panic instead of making callers add an error path.
		panic(fmt.Sprintf(`attempted to write to read-only step "%s"`, step))
	}

	stepdir := filepath.Join(c.stateDir, step)
	return StepWriter{stepdir: stepdir}
}

type StepWriter struct {
	stepdir string // path to step-specific state directory
}

// FIXME: none of these operations are atomic on the FS; just move the progress
// file from name to name instead
func (sw StepWriter) MarkFailed() error {
	err := utils.System.Remove(filepath.Join(sw.stepdir, file.InProgress))
	if err != nil {
		return err
	}

	_, err = utils.System.OpenFile(path.Join(sw.stepdir, file.Failed), os.O_CREATE, 0700)
	if err != nil {
		return err
	}

	return nil
}

func (sw StepWriter) MarkComplete() error {
	err := utils.System.Remove(filepath.Join(sw.stepdir, file.InProgress))
	if err != nil {
		return err
	}

	_, err = utils.System.OpenFile(path.Join(sw.stepdir, file.Complete), os.O_CREATE, 0700)
	if err != nil {
		return err
	}

	return nil
}

func (sw StepWriter) MarkInProgress() error {
	_, err := utils.System.OpenFile(path.Join(sw.stepdir, file.InProgress), os.O_CREATE, 0700)
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
