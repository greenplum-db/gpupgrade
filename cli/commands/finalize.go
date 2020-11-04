//  Copyright (c) 2017-2020 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package commands

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

func finalize() *cobra.Command {
	var verbose bool
	var automatic bool

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

			st, err := commanders.NewStep(idl.Step_FINALIZE,
				&step.BufferedStreams{},
				verbose,
				automatic,
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

			st.RunCLISubstep(idl.Substep_STOP_HUB_AND_AGENTS, func(streams step.OutStreams) error {
				return stopHubAndAgents(false)
			})

			st.RunCLISubstep(idl.Substep_DELETE_MASTER_STATEDIR, func(streams step.OutStreams) error {
				// Removing the state directory removes the step status file.
				// Disable the store so the step framework does not try to write
				// to a non-existent status file.
				st.DisableStore()
				return upgrade.DeleteDirectories([]string{utils.GetStateDir()}, upgrade.StateDirectoryFiles, streams)
			})

			return st.Complete(fmt.Sprintf(`
Finalize completed successfully.

The target cluster is now ready to use, running Greenplum %s.
PGPORT: %d
MASTER_DATA_DIRECTORY: %s

NEXT ACTIONS
------------
Run the “complete” data migration scripts, and recreate any additional tables,
indexes, and roles that were dropped or altered to resolve migration issues.`,
				response.GetTargetVersion(), response.GetTarget().GetPort(), response.GetTarget().GetMasterDataDirectory()))
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print the output stream from all substeps")
	cmd.Flags().BoolVarP(&automatic, "automatic", "a", false, "do not prompt for confirmation to proceed")
	cmd.Flags().MarkHidden("automatic") //nolint

	return addHelpToCommand(cmd, FinalizeHelp)
}
