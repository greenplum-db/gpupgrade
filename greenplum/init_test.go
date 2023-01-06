// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
)

// Does nothing.
func Success() {}

func FailedMain() {
	os.Exit(1)
}

func IsPostmasterRunningCmd_MatchesNoProcesses() {
	os.Exit(1)
}

func IsPostmasterRunningCmd_Errors() {
	os.Stderr.WriteString("exit status 2")
	os.Exit(2)
}

// Prints the environment, one variable per line, in NAME=VALUE format.
func EnvironmentMain() {
	for _, e := range os.Environ() {
		fmt.Println(e)
	}
}

func init() {
	exectest.RegisterMains(
		Success,
		FailedMain,
		IsPostmasterRunningCmd_MatchesNoProcesses,
		IsPostmasterRunningCmd_Errors,
		EnvironmentMain,
	)
}

// Enable exectest.NewCommand mocking.
func TestMain(m *testing.M) {
	os.Exit(exectest.Run(m))
}
