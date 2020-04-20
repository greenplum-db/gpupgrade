// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/hub"
)

func TestUpgradeStandby(t *testing.T) {
	testhelper.SetupTestLogger()

	t.Run("it upgrades the standby through gpinitstandby", func(t *testing.T) {
		config := hub.StandbyConfig{
			Port:          8888,
			Hostname:      "some-hostname",
			DataDirectory: "/some/standby/data/directory",
		}

		runner := newSpyRunner()
		err := hub.UpgradeStandby(runner, config)
		if err != nil {
			t.Errorf("unexpected error: %+v", err)
		}

		if runner.TimesRunWasCalledWith("gpinitstandby") != 2 {
			t.Errorf("got %v calls to config.Run, wanted %v calls",
				runner.TimesRunWasCalledWith("gpinitstandby"),
				2)
		}

		if !runner.Call("gpinitstandby", 1).ArgumentsInclude("-r") {
			t.Errorf("expected remove to have been called")
		}

		if !runner.Call("gpinitstandby", 1).ArgumentsInclude("-a") {
			t.Errorf("expected remove to have been called without user prompt")
		}

		portArgument := runner.
			Call("gpinitstandby", 2).
			ArgumentValue("-P")

		hostnameArgument := runner.
			Call("gpinitstandby", 2).
			ArgumentValue("-s")

		dataDirectoryArgument := runner.
			Call("gpinitstandby", 2).
			ArgumentValue("-S")

		automaticArgument := runner.
			Call("gpinitstandby", 2).
			ArgumentsInclude("-a")

		if portArgument != "8888" {
			t.Errorf("got port for new standby = %v, wanted %v",
				portArgument, "8888")
		}

		if hostnameArgument != "some-hostname" {
			t.Errorf("got hostname for new standby = %v, wanted %v",
				hostnameArgument, "some-hostname")
		}

		if dataDirectoryArgument != "/some/standby/data/directory" {
			t.Errorf("got standby data directory for new standby = %v, wanted %v",
				dataDirectoryArgument, "/some/standby/data/directory")
		}

		if !automaticArgument {
			t.Error("got automatic argument to be set, it was not")
		}
	})
}

type spyRunner struct {
	calls map[string][]*spyCall
}

type spyCall struct {
	arguments []string
}

func newSpyRunner() *spyRunner {
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
