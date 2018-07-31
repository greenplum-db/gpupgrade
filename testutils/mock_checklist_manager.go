package testutils

import (
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
)

type MockChecklistManager struct {
	mapComplete   map[string]bool
	mapFailed     map[string]bool
	mapInProgress map[string]bool
	mapReset      map[string]bool
	loadedNames   []string
	loadedCodes   map[string]pb.UpgradeSteps
}

func NewMockChecklistManager() *MockChecklistManager {
	return &MockChecklistManager{
		mapComplete:   make(map[string]bool, 0),
		mapFailed:     make(map[string]bool, 0),
		mapInProgress: make(map[string]bool, 0),
		mapReset:      make(map[string]bool, 0),
		loadedNames:   make([]string, 0),
		loadedCodes:   make(map[string]pb.UpgradeSteps, 0),
	}
}

func (cm *MockChecklistManager) GetStepReader(step string) upgradestatus.StateReader {
	return MockStepReader{step: step, code: cm.loadedCodes[step], manager: cm}
}

func (cm *MockChecklistManager) AddStep(name string, code pb.UpgradeSteps) {
	cm.loadedNames = append(cm.loadedNames, name)
	cm.loadedCodes[name] = code
}

// Use AddStep() to store the list of steps that this mock should return from
// AllSteps().
func (cm *MockChecklistManager) AllSteps() []upgradestatus.StateReader {
	steps := make([]upgradestatus.StateReader, len(cm.loadedNames))
	for i, name := range cm.loadedNames {
		steps[i] = cm.GetStepReader(name)
	}
	return steps
}

func (cm *MockChecklistManager) GetStepWriter(step string) upgradestatus.StateWriter {
	return MockStepWriter{step: step, manager: cm}
}

type MockStepReader struct {
	step    string
	code    pb.UpgradeSteps
	manager *MockChecklistManager
}

func (r MockStepReader) Status() pb.StepStatus {
	switch {
	case r.manager.IsPending(r.step):
		return pb.StepStatus_PENDING
	case r.manager.IsInProgress(r.step):
		return pb.StepStatus_RUNNING
	case r.manager.IsComplete(r.step):
		return pb.StepStatus_COMPLETE
	case r.manager.IsFailed(r.step):
		return pb.StepStatus_FAILED
	default:
		panic("unexpected step state in MockChecklistManager")
	}
}

func (r MockStepReader) Name() string {
	return r.step
}

func (r MockStepReader) Code() pb.UpgradeSteps {
	return r.code
}

type MockStepWriter struct {
	step    string
	manager *MockChecklistManager
}

func (w MockStepWriter) MarkComplete() error {
	w.manager.mapComplete[w.step] = true
	return nil
}

func (w MockStepWriter) MarkInProgress() error {
	w.manager.mapInProgress[w.step] = true
	return nil
}

func (w MockStepWriter) MarkFailed() error {
	w.manager.mapFailed[w.step] = true
	return nil
}

func (w MockStepWriter) ResetStateDir() error {
	w.manager.mapReset[w.step] = true
	w.manager.mapComplete[w.step] = false
	w.manager.mapFailed[w.step] = false
	w.manager.mapInProgress[w.step] = false
	return nil
}

// Check that nothing has happened yet
func (cm *MockChecklistManager) IsPending(step string) bool {
	return !cm.mapComplete[step] && !cm.mapFailed[step] && !cm.mapInProgress[step]
}

// Check that the step was running and is now complete
func (cm *MockChecklistManager) IsComplete(step string) bool {
	return cm.mapComplete[step] && !cm.mapFailed[step] && cm.mapInProgress[step]
}

// Check that the step was running and has now failed
func (cm *MockChecklistManager) IsFailed(step string) bool {
	return !cm.mapComplete[step] && cm.mapFailed[step] && cm.mapInProgress[step]
}

// Check that the step is running
func (cm *MockChecklistManager) IsInProgress(step string) bool {
	return !cm.mapComplete[step] && !cm.mapFailed[step] && cm.mapInProgress[step]
}

// Check that the state dir for the step was reset at some point
func (cm *MockChecklistManager) WasReset(step string) bool {
	return cm.mapReset[step]
}
