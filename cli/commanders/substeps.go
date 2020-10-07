//  Copyright (c) 2017-2020 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package commanders

import (
	"github.com/greenplum-db/gpupgrade/idl"
)

type substepText struct {
	OutputText string
	HelpText   string
}

var SubstepDescriptions = map[idl.Substep]substepText{
	idl.Substep_CREATING_DIRECTORIES:                     substepText{"Creating directories...", "Create directories"},
	idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG:             substepText{"Saving source cluster configuration...", "Save source cluster configuration"},
	idl.Substep_START_HUB:                                substepText{"Starting gpupgrade hub process...", "Start gpupgrade hub process"},
	idl.Substep_START_AGENTS:                             substepText{"Starting gpupgrade agent processes...", "Start gpupgrade agent processes"},
	idl.Substep_CHECK_DISK_SPACE:                         substepText{"Checking disk space...", "Check disk space"},
	idl.Substep_CREATE_TARGET_CONFIG:                     substepText{"Generating target cluster configuration...", "Generate target cluster configuration"},
	idl.Substep_INIT_TARGET_CLUSTER:                      substepText{"Creating target cluster...", "Create target cluster"},
	idl.Substep_SHUTDOWN_TARGET_CLUSTER:                  substepText{"Stopping target cluster...", "Stop target cluster"},
	idl.Substep_BACKUP_TARGET_MASTER:                     substepText{"Backing up target master...", "Back up target master"},
	idl.Substep_CHECK_UPGRADE:                            substepText{"Running pg_upgrade checks...", "Run pg_upgrade checks"},
	idl.Substep_SHUTDOWN_SOURCE_CLUSTER:                  substepText{"Stopping source cluster...", "Stop source cluster"},
	idl.Substep_UPGRADE_MASTER:                           substepText{"Upgrading master...", "Upgrade master"},
	idl.Substep_COPY_MASTER:                              substepText{"Copying master catalog to primary segments...", "Copy master catalog to primary segments"},
	idl.Substep_UPGRADE_PRIMARIES:                        substepText{"Upgrading primary segments...", "Upgrade primary segments"},
	idl.Substep_START_TARGET_CLUSTER:                     substepText{"Starting target cluster...", "Start target cluster"},
	idl.Substep_UPDATE_TARGET_CATALOG_AND_CLUSTER_CONFIG: substepText{"Updating target master catalog...", "Update target master catalog"},
	idl.Substep_UPDATE_DATA_DIRECTORIES:                  substepText{"Updating data directories...", "Update data directories"},
	idl.Substep_UPDATE_TARGET_CONF_FILES:                 substepText{"Updating target master configuration files...", "Update target master configuration files"},
	idl.Substep_UPGRADE_STANDBY:                          substepText{"Upgrading standby master...", "Upgrade standby master"},
	idl.Substep_UPGRADE_MIRRORS:                          substepText{"Upgrading mirror segments...", "Upgrade mirror segments"},
	idl.Substep_DELETE_TABLESPACES:                       substepText{"Deleting target tablespace directories...", "Delete target tablespace directories"},
	idl.Substep_DELETE_TARGET_CLUSTER_DATADIRS:           substepText{"Deleting target cluster data directories...", "Delete target cluster data directories"},
	idl.Substep_DELETE_SEGMENT_STATEDIRS:                 substepText{"Deleting state directories on the segments...", "Delete state directories on the segments"},
	idl.Substep_STOP_HUB_AND_AGENTS:                      substepText{"Stopping hub and agents...", "Stop hub and agents"},
	idl.Substep_DELETE_MASTER_STATEDIR:                   substepText{"Deleting master state directory...", "Delete master state directory"},
	idl.Substep_ARCHIVE_LOG_DIRECTORIES:                  substepText{"Archiving log directories...", "Archive log directories"},
	idl.Substep_RESTORE_SOURCE_CLUSTER:                   substepText{"Restoring source cluster...", "Restore source cluster"},
	idl.Substep_START_SOURCE_CLUSTER:                     substepText{"Starting source cluster...", "Start source cluster"},
	idl.Substep_RESTORE_PGCONTROL:                        substepText{"Re-enabling source cluster...", "Re-enable source cluster"},
	idl.Substep_RECOVERSEG_SOURCE_CLUSTER:                substepText{"Recovering source cluster mirrors...", "Recover source cluster mirrors"},
}
