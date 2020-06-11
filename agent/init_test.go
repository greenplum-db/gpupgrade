// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"os"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
)

// TODO: migrate to a shared exectest implementation of the simple
// success/failure cases.

// Does nothing.
func Success() {}

func FailedMain() {
	os.Exit(1)
}

func FailedRsync() {
	os.Stderr.WriteString("rsync failed cause I said so")
	os.Exit(2)
}

func init() {
	exectest.RegisterMains(
		Success,
		FailedMain,
		FailedRsync,
	)
}

//
// Override internals of the agent package
//
func SetExecCommand(command exectest.Command) {
	execCommand = command
}
