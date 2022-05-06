// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"os"
	"os/exec"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
)

func init() {
	ResetExecCommand()

	exectest.RegisterMains(
		Success,
		Failure,
	)
}

func Success() {}

func Failure() {
	os.Stderr.WriteString(os.ErrPermission.Error())
	os.Exit(1)
}

var ExecCommand = exec.Command

func SetExecCommand(cmdFunc exectest.Command) {
	ExecCommand = cmdFunc
}

func ResetExecCommand() {
	ExecCommand = nil
}

func TestMain(m *testing.M) {
	os.Exit(exectest.Run(m))
}
