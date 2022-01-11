//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package commands

const initializeConfirmationText = `
You are about to initialize a major-version upgrade of Greenplum.
This should be done only during a downtime window.

gpupgrade initialize will perform a series of steps, including:
 - Check disk space
 - Create the target cluster
 - Run pg_upgrade consistency checks

gpupgrade log files can be found on all hosts in %s

gpupgrade initialize will use these values from %s
source_master_port:   %d
source_gphome:        %s
target_gphome:        %s
mode:                 %s
disk_free_ratio:      %.1f
use_hba_hostnames:    %t
dynamic_library_path: %s
temp_port_range:      %s
hub_port:             %d
agent_port:           %d

You will still have the opportunity to revert the cluster to its original state 
after this step.

WARNING: Do not perform operations on the cluster until gpupgrade is 
finalized or reverted.

Before proceeding, ensure the following have occurred:
 - Take a backup of the source Greenplum cluster
 - Generate and execute the data migration "pre-initialize" scripts
 - Run gpcheckcat to ensure the source catalog has no inconsistencies
 - Run gpstate -e to ensure the source cluster's segments are up and in preferred roles

To suppress this summary, use the --automatic | -a  flag.
`

const executeConfirmationText = `
You are about to run the "execute" command for a major-version upgrade of Greenplum.
This should be done only during a downtime window.

gpupgrade execute will perform a series of steps, including:
- Upgrade master
- Upgrade primary segments

gpupgrade log files can be found on all hosts in %s

You will still have the opportunity to revert the cluster to its original state
after this step.

WARNING: Do not perform operations on the source cluster until gpupgrade is
finalized or reverted.
`

const finalizeConfirmationText = `
You are about to finalize a major-version upgrade of Greenplum.
This should be done only during a downtime window.

gpupgrade finalize will perform a series of steps, including:
 - Update target master catalog
 - Update data directories
 - Update target master configuration files
 - Upgrade standby master
 - Upgrade mirror segments

gpupgrade log files can be found on all hosts in %s

WARNING: You will not be able to revert the cluster to its original state after this step.

WARNING: Do not perform operations on the source and target clusters until gpupgrade is 
finalized or reverted.
`

const revertConfirmationText = `
You are about to revert this upgrade.
This should be done only during a downtime window.

gpupgrade revert will perform a series of steps, including:
 - Delete target cluster data directories
 - Delete state directories on the segments
 - Delete master state directory
 - Archive log directories
 - Restore source cluster
 - Start source cluster

gpupgrade log files can be found on all hosts in %s

WARNING: You cannot revert if you do not have mirrors & standby configured, and execute has started.

WARNING: Do not perform operations on the source and target clusters until gpupgrade revert
has completed.
`
