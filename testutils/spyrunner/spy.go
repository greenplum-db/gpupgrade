package spyrunner

type spyRunner struct {
	calls map[string][]*spyCall
}

type spyCall struct {
	arguments []string
}

func New() *spyRunner {
	return &spyRunner{
		calls: make(map[string][]*spyCall),
	}
}

// implements GreenplumRunner
func (e *spyRunner) Run(utilityName string, arguments ...string) error {
	if e.calls == nil {
		e.calls = make(map[string][]*spyCall)
	}

	calls := e.calls[utilityName]
	e.calls[utilityName] = append(calls, &spyCall{arguments: arguments})

	return nil
}

func (e *spyRunner) TimesRunWasCalledWith(utilityName string) int {
	return len(e.calls[utilityName])
}

func (e *spyRunner) Call(utilityName string, nthCall int) *spyCall {
	callsToUtility := e.calls[utilityName]

	if len(callsToUtility) == 0 {
		return &spyCall{}
	}

	if len(callsToUtility) >= nthCall-1 {
		return callsToUtility[nthCall-1]
	}

	return &spyCall{}
}

func (c *spyCall) ArgumentsInclude(argName string) bool {
	for _, arg := range c.arguments {
		if argName == arg {
			return true
		}
	}
	return false
}

func (c *spyCall) ArgumentValue(flag string) string {
	for i := 0; i < len(c.arguments)-1; i++ {
		current := c.arguments[i]
		next := c.arguments[i+1]

		if flag == current {
			return next
		}
	}

	return ""
}
