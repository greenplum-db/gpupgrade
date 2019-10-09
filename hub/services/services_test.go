package services

import "github.com/greenplum-db/gpupgrade/testutils/exectest"

// Set it to nil so we don't accidentally execute a command for real during tests
func init() {
	ResetExecCommand()
}

func SetExecCommand(cmdFunc exectest.Command) {
	execCommand = cmdFunc
}

func ResetExecCommand() {
	execCommand = nil
}
