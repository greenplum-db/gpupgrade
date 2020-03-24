package hub_test

import (
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/testutils/spyrunner"
)

func TestUpgradeStandby(t *testing.T) {
	testhelper.SetupTestLogger()

	t.Run("it upgrades the standby through gpinitstandby", func(t *testing.T) {
		config := hub.StandbyConfig{
			Port:          8888,
			Hostname:      "some-hostname",
			DataDirectory: "/some/standby/data/directory",
		}

		runner := spyrunner.New()
		hub.UpgradeStandby(runner, config)

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
