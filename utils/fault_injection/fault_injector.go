package fault_injector

import "sync"

// denotes a injectionPoint that runs an unbounded number of times
const UNLIMITED = -1

var isEnabled bool
var mutex sync.Mutex

type injectionPoint struct {
	name     string
	maxCount int
	curCount int
}

var injections = make(map[string]*injectionPoint)

// On latches the fault injector on...there is no way to disable it.
func On() {
	mutex.Lock()
	defer func() { mutex.Unlock() }()

	isEnabled = true
}

// IsOn returns true if the fault injector is on and false otherwise.
func IsOn() bool {
	mutex.Lock()
	defer func() { mutex.Unlock() }()

	return isEnabled
}

// Insert associates a string with a maxCount number of times the injection point.
// If the maxCount has yet been reached, this function returns true, and then
// the caller can then execute arbitrary code in-line; otherwise, this function
// returns false and the caller is not supposed to do anything.
func Insert(name string, maxCount int) bool {

	if !IsOn() {
		return false
	}

	mutex.Lock()

	var injection *injectionPoint
	{
		var ok bool
		injection, ok = injections[name]
		if !ok {
			injection = &injectionPoint{name: name, maxCount: maxCount}
			injections[name] = injection
		}

		if injection.curCount >= injection.maxCount && injection.maxCount != UNLIMITED {
			mutex.Unlock()
			return false
		}

		injection.curCount++
	}

	mutex.Unlock()

	return true
}
