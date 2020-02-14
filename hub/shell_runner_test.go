package hub_test

type spyRunner struct {
	calls map[string][]*SpyCall
}

type SpyCall struct {
	arguments []string
}

func NewSpyRunner() *spyRunner {
	return &spyRunner{
		calls: make(map[string][]*SpyCall),
	}
}

// implements shell runner
func (e *spyRunner) Run(utilityName string, arguments ...string) error {
	if e.calls == nil {
		e.calls = make(map[string][]*SpyCall)
	}

	calls := e.calls[utilityName]
	e.calls[utilityName] = append(calls, &SpyCall{arguments: arguments})

	return nil
}

func (e *spyRunner) TimesRunWasCalledWith(utilityName string) int {
	return len(e.calls[utilityName])
}

func (e *spyRunner) Call(utilityName string, nthCall int) *SpyCall {
	callsToUtility := e.calls[utilityName]

	if len(callsToUtility) == 0 {
		return &SpyCall{}
	}

	if len(callsToUtility) >= nthCall-1 {
		return callsToUtility[nthCall-1]
	}

	return &SpyCall{}
}

func (c *SpyCall) ArgumentsInclude(argName string) bool {
	for _, arg := range c.arguments {
		if argName == arg {
			return true
		}
	}
	return false
}

func (c *SpyCall) ArgumentValue(flag string) string {
	for i := 0; i < len(c.arguments)-1; i++ {
		current := c.arguments[i]
		next := c.arguments[i+1]

		if flag == current {
			return next
		}
	}

	return ""
}
