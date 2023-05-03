// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

func finalize() *cobra.Command {
	var verbose bool
	var nonInteractive bool

	cmd := &cobra.Command{
		Use:   "finalize",
		Short: "finalizes the cluster after upgrade execution",
		Long:  FinalizeHelp,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			var response idl.FinalizeResponse

			logdir, err := utils.GetLogDir()
			if err != nil {
				return err
			}

			confirmationText := fmt.Sprintf(finalizeConfirmationText,
				cases.Title(language.English).String(idl.Step_finalize.String()),
				finalizeSubsteps, logdir)

			st, err := commanders.Begin(idl.Step_finalize, verbose, nonInteractive, confirmationText)
			if err != nil {
				if errors.Is(err, step.Quit) {
					// If user cancels don't return an error to main to avoid
					// printing "Error:".
					return nil
				}
				return err
			}

			st.RunHubSubstep(func(streams step.OutStreams) error {
				client, err := connectToHub()
				if err != nil {
					return err
				}

				response, err = commanders.Finalize(client, verbose)
				if err != nil {
					return err
				}

				return nil
			})

			st.Run(idl.Substep_stop_hub_and_agents, func(streams step.OutStreams) error {
				return stopHubAndAgents()
			})

			st.RunConditionally(idl.Substep_execute_finalize_data_migration_scripts, !nonInteractive, func(streams step.OutStreams) error {
				fmt.Println()
				fmt.Println()

				currentDir := filepath.Join(response.GetLogArchiveDirectory(), "data-migration-scripts", "current")
				return commanders.ApplyDataMigrationScripts(nonInteractive, response.GetTarget().GetGpHome(), int(response.GetTarget().GetCoordinator().GetPort()),
					response.GetLogArchiveDirectory(), utils.System.DirFS(currentDir), currentDir, idl.Step_finalize)
			})

			st.Run(idl.Substep_delete_master_statedir, func(streams step.OutStreams) error {
				// Removing the state directory removes the step status file.
				// Disable the store so the step framework does not try to write
				// to a non-existent status file.
				st.DisableStore()
				return upgrade.DeleteDirectories([]string{utils.GetStateDir()}, upgrade.StateDirectoryFiles, streams)
			})

			return st.Complete(fmt.Sprintf(`
Finalize completed successfully.

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
"gpupgrade apply --gphome %s --port %d --input-dir %s --phase %s"`,
				response.GetTarget().GetVersion(),
				fmt.Sprintf("%s.<contentID>%s", response.GetUpgradeID(), upgrade.OldSuffix),
				response.GetArchivedSourceCoordinatorDataDirectory(),
				response.GetLogArchiveDirectory(),
				filepath.Join(response.GetTarget().GetGpHome(), "greenplum_path.sh"),
				filepath.Join(filepath.Dir(response.GetTarget().GetGpHome()), "greenplum-db"), response.GetTarget().GetGpHome(),
				filepath.Join(response.GetTarget().GetGpHome(), "greenplum_path.sh"),
				response.GetTarget().GetCoordinator().GetDataDir(),
				response.GetTarget().GetCoordinator().GetPort(),
				idl.Step_finalize,
				response.GetTarget().GetGpHome(), response.GetTarget().GetCoordinator().GetPort(), filepath.Join(response.GetLogArchiveDirectory(), "data-migration-scripts"), idl.Step_finalize,
			))
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print the output stream from all substeps")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "do not prompt for confirmation to proceed")
	cmd.Flags().MarkHidden("non-interactive") //nolint
	return addHelpToCommand(cmd, FinalizeHelp)
}
