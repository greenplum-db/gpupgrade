// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package integrations_test

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/greenplum-db/gpupgrade/cli/commands"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestHelpCommands(t *testing.T) {
	helpMap := map[string]string{
		"":           commands.GlobalHelp,
		"initialize": commands.InitializeHelp,
		"execute":    commands.ExecuteHelp,
		"finalize":   commands.FinalizeHelp,
		"revert":     commands.RevertHelp,
	}

	flagList := []string{"-?", "-h", "--help", "help"}
	for command, help := range helpMap {
		for _, flag := range flagList {
			command := command
			flag := flag
			help := help

			t.Run(fmt.Sprintf("testing command %q with flag %q", command, flag), func(t *testing.T) {
				cmd := exec.Command("gpupgrade", command, flag)
				if command == "" {
					cmd = exec.Command("gpupgrade", flag)
				}
				output, err := cmd.Output()
				if err != nil {
					t.Errorf("unexpected err: %#v", err)
				}

				if string(output) != help {
					t.Errorf("got help output %q want %q", string(output), help)
				}
			})
		}
	}

	t.Run("shows global help when no arguments are passed", func(t *testing.T) {
		cmd := exec.Command("gpupgrade")
		output, err := cmd.Output()
		if err != nil {
			t.Errorf("unexpected err: %#v", err)
		}

		logdir, err := utils.GetLogDir()
		if err != nil {
			t.Errorf("failed to get log dir: %v", err)
		}

		expected := fmt.Sprintf(commands.GlobalHelp, logdir)
		if string(output) != expected {
			t.Errorf("got help output %q want %q", string(output), expected)
		}
	})
}
