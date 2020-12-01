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
	"github.com/greenplum-db/gpupgrade/utils"
)

func execute() *cobra.Command {
	var verbose bool
	var nonInteractive bool

	cmd := &cobra.Command{
		Use:   "execute",
		Short: "executes the upgrade",
		Long:  ExecuteHelp,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			cmd.SilenceUsage = true
			var response idl.ExecuteResponse

			logdir, err := utils.GetLogDir()
			if err != nil {
				return err
			}

			confirmationText := fmt.Sprintf(executeConfirmationText, logdir)

			st, err := commanders.NewStep(idl.Step_EXECUTE,
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

				response, err = commanders.Execute(client, verbose)
				if err != nil {
					return err
				}

				return nil
			})

			return st.Complete(fmt.Sprintf(`
Execute completed successfully.

The target cluster is now running. You may now run queries against the target 
database and perform any other validation desired prior to finalizing your upgrade.
PGPORT: %d
MASTER_DATA_DIRECTORY: %s

WARNING: If any queries modify the target database prior to gpupgrade finalize, 
it will be inconsistent with the source database. 

NEXT ACTIONS
------------
If you are satisfied with the state of the cluster, run "gpupgrade finalize" 
to proceed with the upgrade.

To return the cluster to its original state, run "gpupgrade revert".`,
				response.GetTarget().GetPort(), response.GetTarget().GetMasterDataDirectory()))
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print the output stream from all substeps")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "do not prompt for confirmation to proceed")
	cmd.Flags().MarkHidden("non-interactive") //nolint

	return addHelpToCommand(cmd, ExecuteHelp)
}
