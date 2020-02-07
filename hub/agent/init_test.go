package agent

import (
	"os"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
)

func TestMain(m *testing.M) {
	os.Exit(exectest.Run(m))
}

func SetExecCommand(cmdFunc exectest.Command) {
	execCommand = cmdFunc
}

func ResetExecCommand() {
	execCommand = nil
}

func Gpupgrade_agent_Errors() {
	os.Stderr.WriteString("could not find state-directory")
	os.Exit(1)
}

func Gpupgrade_agent() {
}

func init() {
	exectest.RegisterMains(
		Gpupgrade_agent,
		Gpupgrade_agent_Errors,
	)
}
