package agent

import (
	"os"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
)

func SetExecCommand(cmdFunc exectest.Command) {
	execCommand = cmdFunc
}

func ResetExecCommand() {
	execCommand = nil
}

func TestMain(m *testing.M) {
	os.Exit(exectest.Run(m))
}
