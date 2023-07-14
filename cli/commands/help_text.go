// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

var initializeSubsteps commanders.Substeps
var executeSubsteps commanders.Substeps
var finalizeSubsteps commanders.Substeps
var revertSubsteps commanders.Substeps
var InitializeHelp string
var ExecuteHelp string
var FinalizeHelp string
var RevertHelp string
var GlobalHelp string
var Help map[idl.Step]string

func init() {
	logDir, err := utils.GetLogDir()
	if err != nil {
		panic(fmt.Sprintf("failed to get log directory: %v", err))
	}

	initializeSubsteps = commanders.Substeps{
		idl.Substep_verify_gpdb_versions,
		idl.Substep_saving_source_cluster_config,
		idl.Substep_start_hub,
		idl.Substep_generate_data_migration_scripts,
		idl.Substep_execute_stats_data_migration_scripts,
		idl.Substep_execute_initialize_data_migration_scripts,
		idl.Substep_verify_gpupgrade_is_installed_across_all_hosts,
		idl.Substep_start_agents,
		idl.Substep_check_environment,
		idl.Substep_create_backupdirs,
		idl.Substep_check_disk_space,
		idl.Substep_generate_target_config,
		idl.Substep_init_target_cluster,
		idl.Substep_setting_dynamic_library_path_on_target_cluster,
		idl.Substep_shutdown_target_cluster,
		idl.Substep_backup_target_master,
		idl.Substep_initialize_wait_for_cluster_to_be_ready,
		idl.Substep_check_upgrade,
	}

	executeSubsteps = commanders.Substeps{
		idl.Substep_ensure_gpupgrade_agents_are_running,
		idl.Substep_check_active_connections_on_source_cluster,
		idl.Substep_wait_for_cluster_to_be_ready_before_upgrade_master,
		idl.Substep_shutdown_source_cluster,
		idl.Substep_upgrade_master,
		idl.Substep_copy_master,
		idl.Substep_upgrade_primaries,
		idl.Substep_start_target_cluster,
	}

	finalizeSubsteps = commanders.Substeps{
		idl.Substep_ensure_gpupgrade_agents_are_running,
		idl.Substep_check_active_connections_on_target_cluster,
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
		idl.Substep_delete_backupdir,
		idl.Substep_delete_segment_statedirs,
		idl.Substep_stop_hub_and_agents,
		idl.Substep_execute_finalize_data_migration_scripts,
		idl.Substep_analyze_target_cluster,
		idl.Substep_delete_master_statedir,
	}

	revertSubsteps = commanders.Substeps{
		idl.Substep_ensure_gpupgrade_agents_are_running,
		idl.Substep_check_active_connections_on_target_cluster,
		idl.Substep_shutdown_target_cluster,
		idl.Substep_delete_target_cluster_datadirs,
		idl.Substep_delete_tablespaces,
		idl.Substep_restore_pgcontrol,
		idl.Substep_restore_source_cluster,
		idl.Substep_start_source_cluster,
		idl.Substep_recoverseg_source_cluster,
		idl.Substep_archive_log_directories,
		idl.Substep_delete_backupdir,
		idl.Substep_delete_segment_statedirs,
		idl.Substep_stop_hub_and_agents,
		idl.Substep_execute_revert_data_migration_scripts,
		idl.Substep_delete_master_statedir,
	}

	InitializeHelp = fmt.Sprintf(initializeHelpText, cases.Title(language.English).String(idl.Step_initialize.String()), initializeSubsteps, logDir)
	ExecuteHelp = fmt.Sprintf(executeHelpText, cases.Title(language.English).String(idl.Step_execute.String()), executeSubsteps, logDir)
	FinalizeHelp = fmt.Sprintf(finalizeHelpText, cases.Title(language.English).String(idl.Step_finalize.String()), finalizeSubsteps, logDir)
	RevertHelp = fmt.Sprintf(revertHelpText, cases.Title(language.English).String(idl.Step_revert.String()), revertSubsteps, logDir)
	GlobalHelp = fmt.Sprintf(globalHelpText, logDir)

	Help = map[idl.Step]string{
		idl.Step_initialize: InitializeHelp,
		idl.Step_execute:    ExecuteHelp,
		idl.Step_finalize:   FinalizeHelp,
		idl.Step_revert:     RevertHelp,
	}
}

const initializeHelpText = `
Runs pre-upgrade checks and prepares the cluster for upgrade.

%s will carry out the following steps:
%s
During or after gpupgrade initialize, you may revert the cluster to its
original state by running gpupgrade revert.

Usage: gpupgrade initialize --file <path/to/config_file>

Required Flags:

  -f, --file      config file containing upgrade parameters
                  (e.g. gpupgrade_config)

Optional Flags:

  -h, --help                 displays help output for initialize
  -v, --verbose              outputs detailed logs for initialize
      --pg-upgrade-verbose   execute pg_upgrade with verbose internal logging. Requires the verbose flag.

gpupgrade log files can be found on all hosts in %s
`
const executeHelpText = `
Upgrades the master and primary segments to the target Greenplum version.
This command should be run only during a downtime window.

%s will carry out the following steps:
%s
During or after gpupgrade execute, you may revert the cluster to its
original state by running gpupgrade revert.

Usage: gpupgrade execute

Optional Flags:

  -h, --help                 displays help output for execute
  -v, --verbose              outputs detailed logs for execute
      --pg-upgrade-verbose   execute pg_upgrade with verbose internal logging. Requires the verbose flag.
      --parent-backup-dir    The parent directory location used internally to store the backup of the 
                             master data directory and user defined master tablespaces. Defaults to the 
                             parent directory of the master data directory such as /data given 
                             /data/master/gpseg-1.

gpupgrade log files can be found on all hosts in %s
`
const finalizeHelpText = `
Upgrades the standby master and mirror segments to the target Greenplum version.
This command should be run only during a downtime window.

%s will carry out the following steps:
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
const revertHelpText = `
Returns the cluster to its original state.
This command cannot be run after gpupgrade finalize has begun.
This command should be run only during a downtime window.

%s will carry out the following steps:
%s
Usage: gpupgrade revert

Optional Flags:

  -h, --help      displays help output for revert
  -v, --verbose   outputs detailed logs for revert

NOTE: After running revert, you must execute data migration scripts. 
Refer to documentation for instructions.

Archived gpupgrade log files can be found on all hosts in %s-<upgradeID>-<timestamp>
`
const generateHelp = `
Generates data migration SQL scripts to resolve catalog inconsistencies between 
the source and target clusters. After which run "gpupgrade apply".
This command does not require downtime.

IMPORTANT: Running the data migration scripts generate takes a snapshot of the 
database. If any new data or objects that cannot be upgraded are created after 
the generator is run, will be missed. In such scenario, re-generate in order 
to detect the new data and objects.

Usage: gpupgrade generate --gphome "$GPHOME" --port "$PGPORT"

Required Flags:

  --gphome       path to the Greenplum installation
  --port         master port for Greenplum cluster

Optional Flags:

  --output-dir    output path to the current generated data migration SQL files. 
                  Defaults to $HOME/gpAdminLogs/gpupgrade/data-migration-scripts
`
const applyHelp = `
Applies data migration SQL scripts to resolve catalog inconsistencies between 
the source and target clusters. First run "gpupgrade generate".
This command may require downtime depending on what scripts are run. See online 
documentation for details.

Usage: gpupgrade apply --gphome "$GPHOME" --port "$PGPORT" --phase initialize

Required Flags:

  --gphome       path to the Greenplum installation
  --port         master port for Greenplum cluster
  --phase        the data migration phase. Either "pre-initialize", 
                 "post-finalize", "post-revert", or "stats".

Optional Flags:

  --input-dir    path to the generated data migration SQL files. 
                 Defaults to $HOME/gpAdminLogs/gpupgrade/data-migration-scripts
`
const ConfigHelp = `
The config subcommand allows one to view configuration parameters only after 
initialize has started. It is useful for starting or connecting to the 
target cluster by getting the target cluster data directory and port parameters.

Usage: gpupgrade config show <flag>

Optional Flags:

--upgrade-id       differentiates the intermediate target cluster directories. 
                   The upgrade IO is also used when archiving the log directories
                   and source cluster data directories after finalize.
--source-gphome
--target-gphome
--target-datadir
--target-port

Example:
  gpupgrade config show --target-datadir
`

const globalHelpText = `
gpupgrade performs an in-place cluster upgrade to the next major version.

Usage: gpupgrade [command] <flags> 

Required Commands:

  1. initialize   runs pre-upgrade checks and prepares the cluster for upgrade

  2. execute      upgrades the master and primary segments to the target
                  Greenplum version

  3. finalize     upgrades the standby master and mirror segments to the target
                  Greenplum version

Optional Commands:

  revert          returns the cluster to its original state
                  Note: revert cannot be used after gpupgrade finalize

  generate        generates data migration SQL scripts

  apply           applies data migration SQL scripts

  config show     shows configuration parameters. 
                  One can only view the configuration parameters only 
                  after initialize has started. The config subcommand is
                  useful for getting the target cluster data directory
                  and port in order to start or connect to the target cluster.

Optional Flags:

  -h, --help      displays help output for gpupgrade
  -v, --verbose   outputs detailed logs for gpupgrade
  -V, --version   displays the version of the current gpupgrade utility

gpupgrade log files can be found on all hosts in %s

Use "gpupgrade [command] --help" for more information about a command.
`

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
