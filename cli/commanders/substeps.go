// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commanders

import (
	"github.com/greenplum-db/gpupgrade/idl"
)

type substepText struct {
	OutputText string
	HelpText   string
}

var SubstepDescriptions = map[idl.Substep]substepText{
	idl.Substep_saving_source_cluster_config:                                  substepText{"Saving source cluster configuration...", "Save source cluster configuration"},
	idl.Substep_start_hub:                                                     substepText{"Starting gpupgrade hub process...", "Start gpupgrade hub process"},
	idl.Substep_start_agents:                                                  substepText{"Starting gpupgrade agent processes...", "Start gpupgrade agent processes"},
	idl.Substep_check_disk_space:                                              substepText{"Checking disk space...", "Check disk space"},
	idl.Substep_generate_target_config:                                        substepText{"Generating target cluster configuration...", "Generate target cluster configuration"},
	idl.Substep_init_target_cluster:                                           substepText{"Creating target cluster...", "Create target cluster"},
	idl.Substep_setting_dynamic_library_path_on_target_cluster:                substepText{"Setting dynamic library path on target cluster...", "Set dynamic library path on target cluster"},
	idl.Substep_shutdown_target_cluster:                                       substepText{"Stopping target cluster...", "Stop target cluster"},
	idl.Substep_backup_target_master:                                          substepText{"Backing up target master...", "Back up target master"},
	idl.Substep_check_upgrade:                                                 substepText{"Running pg_upgrade checks...", "Run pg_upgrade checks"},
	idl.Substep_shutdown_source_cluster:                                       substepText{"Stopping source cluster...", "Stop source cluster"},
	idl.Substep_upgrade_master:                                                substepText{"Upgrading master...", "Upgrade master"},
	idl.Substep_copy_master:                                                   substepText{"Copying master catalog to primary segments...", "Copy master catalog to primary segments"},
	idl.Substep_upgrade_primaries:                                             substepText{"Upgrading primary segments...", "Upgrade primary segments"},
	idl.Substep_start_target_cluster:                                          substepText{"Starting target cluster...", "Start target cluster"},
	idl.Substep_stop_target_cluster:                                           substepText{"Stopping target cluster...", "Stop target cluster"},
	idl.Substep_update_target_catalog:                                         substepText{"Updating target master catalog...", "Update target master catalog"},
	idl.Substep_update_data_directories:                                       substepText{"Updating data directories...", "Update data directories"},
	idl.Substep_update_target_conf_files:                                      substepText{"Updating target master configuration files...", "Update target master configuration files"},
	idl.Substep_upgrade_standby:                                               substepText{"Upgrading standby master...", "Upgrade standby master"},
	idl.Substep_upgrade_mirrors:                                               substepText{"Upgrading mirror segments...", "Upgrade mirror segments"},
	idl.Substep_delete_tablespaces:                                            substepText{"Deleting target tablespace directories...", "Delete target tablespace directories"},
	idl.Substep_delete_target_cluster_datadirs:                                substepText{"Deleting target cluster data directories...", "Delete target cluster data directories"},
	idl.Substep_delete_segment_statedirs:                                      substepText{"Deleting state directories on the segments...", "Delete state directories on the segments"},
	idl.Substep_stop_hub_and_agents:                                           substepText{"Stopping hub and agents...", "Stop hub and agents"},
	idl.Substep_delete_master_statedir:                                        substepText{"Deleting master state directory...", "Delete master state directory"},
	idl.Substep_archive_log_directories:                                       substepText{"Archiving log directories...", "Archive log directories"},
	idl.Substep_restore_source_cluster:                                        substepText{"Restoring source cluster...", "Restore source cluster"},
	idl.Substep_start_source_cluster:                                          substepText{"Starting source cluster...", "Start source cluster"},
	idl.Substep_restore_pgcontrol:                                             substepText{"Re-enabling source cluster...", "Re-enable source cluster"},
	idl.Substep_recoverseg_source_cluster:                                     substepText{"Recovering source cluster mirrors...", "Recover source cluster mirrors"},
	idl.Substep_remove_source_mirrors:                                         substepText{"Removing source cluster data directories and tablespaces to save space...", "Remove source cluster data directories and tablespaces to save space..."},
	idl.Substep_wait_for_cluster_to_be_ready_after_adding_mirrors_and_standby: substepText{"Waiting for cluster to be ready...", "Wait for cluster to be ready"},
	idl.Substep_wait_for_cluster_to_be_ready_after_updating_catalog:           substepText{"Waiting for cluster to be ready...", "Wait for cluster to be ready"},
}
