package hub_test

import (
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/hub"
)

type spyGreenplumRunner struct {
	hub.ShellRunner
}

func TestUpgradeStandby(t *testing.T) {
	testhelper.SetupTestLogger()

	t.Run("it upgrades the standby through gpinitstandby", func(t *testing.T) {
		hub.ResetUpgradeStandby()

		spyRunner := NewSpyRunner()

		config := hub.StandbyConfig{
			Port:          8888,
			Hostname:      "some-hostname",
			DataDirectory: "/some/standby/data/directory",
		}

		greenplumRunner := &spyGreenplumRunner{spyRunner}
		hub.UpgradeStandby(greenplumRunner, config)

		if spyRunner.TimesRunWasCalledWith("gpinitstandby") != 2 {
			t.Errorf("got %v calls to config.Run, wanted %v calls",
				spyRunner.TimesRunWasCalledWith("gpinitstandby"),
				2)
		}

		if !spyRunner.Call("gpinitstandby", 1).ArgumentsInclude("-r") {
			t.Errorf("expected remove to have been called")
		}

		portArgument := spyRunner.
			Call("gpinitstandby", 2).
			ArgumentValue("-P")

		hostnameArgument := spyRunner.
			Call("gpinitstandby", 2).
			ArgumentValue("-s")

		dataDirectoryArgument := spyRunner.
			Call("gpinitstandby", 2).
			ArgumentValue("-S")

		automaticArgument := spyRunner.
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

func (e *spyGreenplumRunner) BinDir() string {
	return ""
}

func (e *spyGreenplumRunner) MasterDataDirectory() string {
	return ""
}

func (e *spyGreenplumRunner) MasterPort() int {
	return 9999
}
