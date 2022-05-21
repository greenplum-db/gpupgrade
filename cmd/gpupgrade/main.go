// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"

	"github.com/greenplum-db/gpupgrade/cli/commands"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/daemon"
	"github.com/greenplum-db/gpupgrade/utils/logger"
)

func main() {
	debug.SetTraceback("all")

	logger.Initialize("cli")
	defer logger.WritePanics()

	root := commands.BuildRootCommand()
	// Silence usage since Cobra prints usage for all errors rather than just
	// "unknown flag" errors.
	root.SilenceUsage = true

	err := root.Execute()
	if err != nil && err != daemon.ErrSuccessfullyDaemonized {
		if strings.HasPrefix(err.Error(), "unknown flag") {
			cmd := os.Args[1]
			fmt.Println(commands.Help[cmd])
		}

		// We use gplog.Debug instead of Error so the error is not displayed
		// twice to the user in the terminal.
		log.Printf("%+v", err)

		// Print any additional actions that should be taken by the user.
		var actions utils.NextActionErr
		if errors.As(err, &actions) {
			fmt.Print(actions.Help())
		}

		os.Exit(1)
	}
}
