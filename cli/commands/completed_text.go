// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import "github.com/fatih/color"

var InitializeCompletedText = `
%s
NEXT ACTIONS
------------
To proceed with the upgrade, run "gpupgrade execute --verbose"
followed by "gpupgrade finalize --verbose".

To return the cluster to its original state, run "gpupgrade revert --verbose".`

var ExecuteCompletedText = `
The target cluster is now running. You may now run queries against the target 
database and perform any other validation desired prior to finalizing your upgrade.
source %s
export MASTER_DATA_DIRECTORY=%s
export PGPORT=%d
` + color.RedString(`
WARNING: If any queries modify the target database prior to gpupgrade finalize, 
it will be inconsistent with the source database.`) + `

NEXT ACTIONS
------------
If you are satisfied with the state of the cluster, run "gpupgrade finalize --verbose" 
to proceed with the upgrade.

To return the cluster to its original state, run "gpupgrade revert --verbose".`

var FinalizeCompletedText = `
The target cluster has been upgraded to Greenplum %s

The source cluster is not running. If copy mode was used you may start 
the source cluster, but not at the same time as the target cluster. 
To do so configure different ports to avoid conflicts. 

You may delete the source cluster to recover space from all hosts. 
All source cluster data directories end in "%s".
MASTER_DATA_DIRECTORY=%s

The gpupgrade logs can be found on the master and segment hosts in
%s

NEXT ACTIONS
------------
To use the upgraded cluster:
1. Update any scripts to source %s
2. If applicable, update the greenplum-db symlink to point to the target 
   install location: %s -> %s
3. In a new shell:
   source %s
   export MASTER_DATA_DIRECTORY=%s
   export PGPORT=%d
   
   And connect to the database

If you have not already, execute the “%s” data migration scripts with
"gpupgrade apply --gphome %s --port %d --input-dir %s --phase %s"

If you postponed creating optimizer statistics run
"vacuumdb --all --analyze-in-stages"`

var RevertCompletedText = `
The source cluster is now running version %s.
source %s
export MASTER_DATA_DIRECTORY=%s
export PGPORT=%d

The gpupgrade logs can be found on the master and segment hosts in
%s

NEXT ACTIONS
------------
If you have not already, execute the “%s” data migration scripts with
"gpupgrade apply --gphome %s --port %d --input-dir %s --phase %s"

To restart the upgrade, run "gpupgrade initialize --verbose" again.`
