// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum_test

import (
	"fmt"
	"os"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
)

// Does nothing.
func Success() {}

func FailedMain() {
	os.Exit(1)
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
		EnvironmentMain,
	)
}
