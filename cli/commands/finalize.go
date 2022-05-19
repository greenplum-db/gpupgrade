// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

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

			confirmationText := fmt.Sprintf(finalizeConfirmationText, logdir)

			st, err := commanders.NewStep(idl.Step_finalize,
				&step.BufferedStreams{},
				verbose,
				nonInteractive,
				confirmationText,
			)
			if err != nil {
				if errors.Is(err, step.UserCanceled) {
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

			st.RunCLISubstep(idl.Substep_stop_hub_and_agents, func(streams step.OutStreams) error {
				return stopHubAndAgents(false)
			})

			st.RunCLISubstep(idl.Substep_delete_master_statedir, func(streams step.OutStreams) error {
				// Removing the state directory removes the step status file.
				// Disable the store so the step framework does not try to write
				// to a non-existent status file.
				st.DisableStore()
				return upgrade.DeleteDirectories([]string{utils.GetStateDir()}, upgrade.StateDirectoryFiles, streams)
			})

			return st.Complete(fmt.Sprintf(`
Finalize completed successfully.

The target cluster has been upgraded to Greenplum %s:
%s
PGPORT=%d
MASTER_DATA_DIRECTORY=%s

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
3. In a new shell source %s and start the cluster with gpstart.
   Execute the “post-finalize” data migration scripts, and recreate any 
   additional tables, indexes, and roles that were dropped or altered 
   to resolve migration issues.`,
				response.GetTargetVersion(),
				filepath.Join(response.GetTargetCluster().GetGPHome(), "greenplum_path.sh"),
				response.GetTargetCluster().GetPort(),
				response.GetTargetCluster().GetCoordinatorDataDirectory(),
				fmt.Sprintf("%s.<contentID>%s", response.GetUpgradeID(), upgrade.OldSuffix),
				response.GetArchivedSourceCoordinatorDataDirectory(),
				response.GetLogArchiveDirectory(),
				filepath.Join(response.GetTargetCluster().GetGPHome(), "greenplum_path.sh"),
				filepath.Join(filepath.Dir(response.GetTargetCluster().GetGPHome()), "greenplum-db"),
				response.GetTargetCluster().GetGPHome(),
				filepath.Join(response.GetTargetCluster().GetGPHome(), "greenplum_path.sh"),
			))
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print the output stream from all substeps")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "do not prompt for confirmation to proceed")
	cmd.Flags().MarkHidden("non-interactive") //nolint
	return addHelpToCommand(cmd, FinalizeHelp)
}
