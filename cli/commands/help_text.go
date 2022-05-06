// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

var (
	Help           map[string]string
	InitializeHelp string
	ExecuteHelp    string
	FinalizeHelp   string
	RevertHelp     string
)

func init() {
	InitializeHelp = GenerateHelpString(initializeHelp, []idl.Substep{
		idl.Substep_START_HUB,
		idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG,
		idl.Substep_START_AGENTS,
		idl.Substep_CHECK_DISK_SPACE,
		idl.Substep_GENERATE_TARGET_CONFIG,
		idl.Substep_INIT_TARGET_CLUSTER,
		idl.Substep_SETTING_DYNAMIC_LIBRARY_PATH_ON_TARGET_CLUSTER,
		idl.Substep_SHUTDOWN_TARGET_CLUSTER,
		idl.Substep_BACKUP_TARGET_MASTER,
		idl.Substep_CHECK_UPGRADE,
	})
	ExecuteHelp = GenerateHelpString(executeHelp, []idl.Substep{
		idl.Substep_SHUTDOWN_SOURCE_CLUSTER,
		idl.Substep_UPGRADE_MASTER,
		idl.Substep_COPY_MASTER,
		idl.Substep_UPGRADE_PRIMARIES,
		idl.Substep_START_TARGET_CLUSTER,
	})
	FinalizeHelp = GenerateHelpString(finalizeHelp, []idl.Substep{
		idl.Substep_REMOVE_SOURCE_MIRRORS,
		idl.Substep_UPGRADE_MIRRORS,
		idl.Substep_UPGRADE_STANDBY,
		idl.Substep_WAIT_FOR_CLUSTER_TO_BE_READY_AFTER_ADDING_MIRRORS_AND_STANDBY,
		idl.Substep_SHUTDOWN_TARGET_CLUSTER,
		idl.Substep_UPDATE_TARGET_CATALOG,
		idl.Substep_UPDATE_DATA_DIRECTORIES,
		idl.Substep_UPDATE_TARGET_CONF_FILES,
		idl.Substep_START_TARGET_CLUSTER,
		idl.Substep_WAIT_FOR_CLUSTER_TO_BE_READY_AFTER_UPDATING_CATALOG,
		idl.Substep_ARCHIVE_LOG_DIRECTORIES,
		idl.Substep_DELETE_SEGMENT_STATEDIRS,
		idl.Substep_STOP_HUB_AND_AGENTS,
		idl.Substep_DELETE_MASTER_STATEDIR,
		idl.Substep_STOP_TARGET_CLUSTER,
	})
	RevertHelp = GenerateHelpString(revertHelp, []idl.Substep{
		idl.Substep_SHUTDOWN_TARGET_CLUSTER,
		idl.Substep_DELETE_TARGET_CLUSTER_DATADIRS,
		idl.Substep_DELETE_TABLESPACES,
		idl.Substep_RESTORE_PGCONTROL,
		idl.Substep_RESTORE_SOURCE_CLUSTER,
		idl.Substep_START_SOURCE_CLUSTER,
		idl.Substep_RECOVERSEG_SOURCE_CLUSTER,
		idl.Substep_ARCHIVE_LOG_DIRECTORIES,
		idl.Substep_DELETE_SEGMENT_STATEDIRS,
		idl.Substep_STOP_HUB_AND_AGENTS,
		idl.Substep_DELETE_MASTER_STATEDIR,
	})
	Help = map[string]string{
		"initialize": InitializeHelp,
		"execute":    ExecuteHelp,
		"finalize":   FinalizeHelp,
		"revert":     RevertHelp,
	}
}

const initializeHelp = `
Runs pre-upgrade checks and prepares the cluster for upgrade.
This command should be run only during a downtime window.

Initialize will carry out the following steps:
%s
During or after gpupgrade initialize, you may revert the cluster to its
original state by running gpupgrade revert.

Usage: gpupgrade initialize --file <path/to/config_file>

Required Flags:

  -f, --file      config file containing upgrade parameters
                  (e.g. gpupgrade_config)

Optional Flags:

  -a, --automatic   suppress summary & confirmation dialog
  -h, --help        displays help output for initialize
  -v, --verbose     outputs detailed logs for initialize

gpupgrade log files can be found on all hosts in %s
`
const executeHelp = `
Upgrades the master and primary segments to the target Greenplum version.
This command should be run only during a downtime window.

Execute will carry out the following steps:
%s
During or after gpupgrade execute, you may revert the cluster to its
original state by running gpupgrade revert.

Usage: gpupgrade execute

Optional Flags:

  -h, --help      displays help output for execute
  -v, --verbose   outputs detailed logs for execute

gpupgrade log files can be found on all hosts in %s
`
const finalizeHelp = `
Upgrades the standby master and mirror segments to the target Greenplum version.
This command should be run only during a downtime window.

Finalize will carry out the following steps:
%s
Once you run gpupgrade finalize, you may NOT revert the cluster to its
original state.

Usage: gpupgrade finalize

Optional Flags:

  -h, --help      displays help output for finalize
  -v, --verbose   outputs detailed logs for finalize

NOTE: After running finalize, you must execute data migration scripts. 
Refer to documentation for instructions.

gpupgrade log files can be found on all hosts in %s
`
const revertHelp = `
Returns the cluster to its original state.
This command cannot be run after gpupgrade finalize has begun.
This command should be run only during a downtime window.

Revert will carry out some or all of the following steps:
%s
Usage: gpupgrade revert

Optional Flags:

  -h, --help      displays help output for revert
  -v, --verbose   outputs detailed logs for revert

NOTE: After running revert, you must execute data migration scripts. 
Refer to documentation for instructions.

Archived gpupgrade log files can be found on all hosts in %s-<upgradeID>-<timestamp>
`
const GlobalHelp = `
gpupgrade performs an in-place cluster upgrade to the next major version.

NOTE: Before running gpupgrade, you must prepare the cluster. This includes
generating and executing data migration scripts. Refer to documentation 
for instructions.

Usage: gpupgrade [command] <flags> 

Required Commands: Run the three commands in this order

  1. initialize   runs pre-upgrade checks and prepares the cluster for upgrade

                  Usage: gpupgrade initialize --file </path/to/config_file>

                  Required Flags:
                    -f, --file   config file containing upgrade parameters
                                 (e.g. gpupgrade_config)

                  Optional Flags:
                    -a, --automatic   suppress summary & confirmation dialog

  2. execute      upgrades the master and primary segments to the target
                  Greenplum version

  3. finalize     upgrades the standby master and mirror segments to the target
                  Greenplum version

Optional Commands:

  revert          returns the cluster to its original state
                  Note: revert cannot be used after gpupgrade finalize

Optional Flags:

  -h, --help      displays help output for gpupgrade
  -v, --verbose   outputs detailed logs for gpupgrade
  -V, --version   displays the version of the current gpupgrade utility

gpupgrade log files can be found on all hosts in %s

Use "gpupgrade [command] --help" for more information about a command.
`

func GenerateHelpString(baseString string, commandList []idl.Substep) string {
	var formattedList string
	for _, substep := range commandList {
		formattedList += fmt.Sprintf(" - %s\n", commanders.SubstepDescriptions[substep].HelpText)
	}

	logdir, err := utils.GetLogDir()
	if err != nil {
		panic(fmt.Sprintf("failed to get log directory: %v", err))
	}

	return fmt.Sprintf(baseString, formattedList, logdir)
}

// Cobra has multiple ways to handle help text, so we want to force all of them to use the same help text
func addHelpToCommand(cmd *cobra.Command, help string) *cobra.Command {
	// Add a "-?" flag, which Cobra does not provide by default
	var savedPreRunE func(cmd *cobra.Command, args []string) error
	var savedPreRun func(cmd *cobra.Command, args []string)
	if cmd.PreRunE != nil {
		savedPreRunE = cmd.PreRunE
	} else if cmd.PreRun != nil {
		savedPreRun = cmd.PreRun
	}

	var questionHelp bool
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if questionHelp {
			fmt.Print(help)
			os.Exit(0)
		}
		if savedPreRunE != nil {
			return savedPreRunE(cmd, args)
		} else if savedPreRun != nil {
			savedPreRun(cmd, args)
		}
		return nil
	}
	cmd.Flags().BoolVarP(&questionHelp, "?", "?", false, "displays help output")

	// Override the built-in "help" subcommand
	cmd.AddCommand(&cobra.Command{
		Use:   "help",
		Short: "",
		Long:  "",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Print(help)
			return nil
		},
	})
	cmd.SetUsageTemplate(help)

	// Override the built-in "-h" and "--help" flags
	cmd.SetHelpFunc(func(cmd *cobra.Command, strs []string) {
		fmt.Print(help)
	})

	return cmd
}
