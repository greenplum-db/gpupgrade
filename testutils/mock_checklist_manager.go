package testutils

type MockChecklistManager struct {
	mapComplete   map[string]bool
	mapFailed     map[string]bool
	mapInProgress map[string]bool
	mapReset      map[string]bool
}

func NewMockChecklistManager() *MockChecklistManager {
	return &MockChecklistManager{
		mapComplete:   make(map[string]bool, 0),
		mapFailed:     make(map[string]bool, 0),
		mapInProgress: make(map[string]bool, 0),
		mapReset:      make(map[string]bool, 0),
	}
}

func (cm *MockChecklistManager) MarkComplete(step string) error {
	cm.mapComplete[step] = true
	return nil
}

func (cm *MockChecklistManager) MarkInProgress(step string) error {
	cm.mapInProgress[step] = true
	return nil
}

func (cm *MockChecklistManager) MarkFailed(step string) error {
	cm.mapFailed[step] = true
	return nil
}

func (cm *MockChecklistManager) ResetStateDir(step string) error {
	cm.mapReset[step] = true
	cm.mapComplete[step] = false
	cm.mapFailed[step] = false
	cm.mapInProgress[step] = false
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
