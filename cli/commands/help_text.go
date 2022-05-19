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
		idl.Substep_start_hub,
		idl.Substep_saving_source_cluster_config,
		idl.Substep_start_agents,
		idl.Substep_check_disk_space,
		idl.Substep_generate_target_config,
		idl.Substep_init_target_cluster,
		idl.Substep_setting_dynamic_library_path_on_target_cluster,
		idl.Substep_shutdown_target_cluster,
		idl.Substep_backup_target_master,
		idl.Substep_check_upgrade,
	})
	ExecuteHelp = GenerateHelpString(executeHelp, []idl.Substep{
		idl.Substep_shutdown_source_cluster,
		idl.Substep_upgrade_master,
		idl.Substep_copy_master,
		idl.Substep_upgrade_primaries,
		idl.Substep_start_target_cluster,
	})
	FinalizeHelp = GenerateHelpString(finalizeHelp, []idl.Substep{
		idl.Substep_remove_source_mirrors,
		idl.Substep_upgrade_mirrors,
		idl.Substep_upgrade_standby,
		idl.Substep_wait_for_cluster_to_be_ready_after_adding_mirrors_and_standby,
		idl.Substep_shutdown_target_cluster,
		idl.Substep_update_target_catalog,
		idl.Substep_update_data_directories,
		idl.Substep_update_target_conf_files,
		idl.Substep_start_target_cluster,
		idl.Substep_wait_for_cluster_to_be_ready_after_updating_catalog,
		idl.Substep_archive_log_directories,
		idl.Substep_delete_segment_statedirs,
		idl.Substep_stop_hub_and_agents,
		idl.Substep_delete_master_statedir,
		idl.Substep_stop_target_cluster,
	})
	RevertHelp = GenerateHelpString(revertHelp, []idl.Substep{
		idl.Substep_shutdown_target_cluster,
		idl.Substep_delete_target_cluster_datadirs,
		idl.Substep_delete_tablespaces,
		idl.Substep_restore_pgcontrol,
		idl.Substep_restore_source_cluster,
		idl.Substep_start_source_cluster,
		idl.Substep_recoverseg_source_cluster,
		idl.Substep_archive_log_directories,
		idl.Substep_delete_segment_statedirs,
		idl.Substep_stop_hub_and_agents,
		idl.Substep_delete_master_statedir,
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
