// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	_ "github.com/lib/pq"

	"github.com/greenplum-db/gpupgrade/cli/commands"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/daemon"
)

func main() {
	debug.SetTraceback("all")
	logdir, err := utils.GetLogDir()
	if err != nil {
		fmt.Printf("\n%+v\n", err)
		os.Exit(1)
	}
	gplog.InitializeLogging("gpupgrade_cli", logdir)

	root := commands.BuildRootCommand()
	root.SilenceErrors = true // we'll print these ourselves

	err = root.Execute()
	if err != nil && err != daemon.ErrSuccessfullyDaemonized {
		// Use v to print the stack trace of an object errors.
		fmt.Printf("\n%+v\n", err)
		os.Exit(1)
	}
}
