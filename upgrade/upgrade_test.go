package upgrade

import (
	"os"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
)

func init() {
	ResetExecCommand()
}

func SetExecCommand(cmdFunc exectest.Command) {
	execCommand = cmdFunc
}

func ResetExecCommand() {
	execCommand = nil
}

// NewOptionList is a public version of upgrade.newOptionList for testing
// purposes.
func NewOptionList(opts []Option) *optionList {
	return newOptionList(opts)
}

func TestMain(m *testing.M) {
	os.Exit(exectest.Run(m))
}
