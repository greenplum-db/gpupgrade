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
	"github.com/greenplum-db/gpupgrade/config"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

func execute() *cobra.Command {
	var verbose bool
	var pgUpgradeVerbose bool
	var nonInteractive bool
	var parentBackupDirs string

	cmd := &cobra.Command{
		Use:   "execute",
		Short: "executes the upgrade",
		Long:  ExecuteHelp,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			cmd.SilenceUsage = true
			var response idl.ExecuteResponse

			if cmd.Flag("pg-upgrade-verbose").Changed && !cmd.Flag("verbose").Changed {
				return fmt.Errorf("expected --verbose when using --pg-upgrade-verbose")
			}

			conf, err := config.Read()
			if err != nil {
				return err
			}

			revertWarning := ""
			if !conf.Source.HasAllMirrorsAndStandby() && conf.Mode == idl.Mode_link {
				revertWarning = revertWarningText
			}

			logdir, err := utils.GetLogDir()
			if err != nil {
				return err
			}

			confirmationText := fmt.Sprintf(executeConfirmationText, revertWarning,
				cases.Title(language.English).String(idl.Step_execute.String()),
				executeSubsteps, logdir)

			st, err := commanders.NewStep(idl.Step_execute,
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

				request := &idl.ExecuteRequest{
					PgUpgradeVerbose: pgUpgradeVerbose,
					ParentBackupDirs: parentBackupDirs,
				}
				response, err = commanders.Execute(client, request, verbose)
				if err != nil {
					return err
				}

				return nil
			})

			return st.Complete(fmt.Sprintf(`
Execute completed successfully.

The target cluster is now running. You may now run queries against the target 
database and perform any other validation desired prior to finalizing your upgrade.
source %s
export MASTER_DATA_DIRECTORY=%s
export PGPORT=%d

WARNING: If any queries modify the target database prior to gpupgrade finalize, 
it will be inconsistent with the source database. 

NEXT ACTIONS
------------
If you are satisfied with the state of the cluster, run "gpupgrade finalize" 
to proceed with the upgrade.

To return the cluster to its original state, run "gpupgrade revert".`,
				filepath.Join(response.GetTarget().GetGPHome(), "greenplum_path.sh"),
				response.GetTarget().GetCoordinatorDataDirectory(),
				response.GetTarget().GetPort()))
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print the output stream from all substeps")
	cmd.Flags().BoolVar(&pgUpgradeVerbose, "pg-upgrade-verbose", false, "execute pg_upgrade with --verbose")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "do not prompt for confirmation to proceed")
	cmd.Flags().MarkHidden("non-interactive") //nolint
	cmd.Flags().StringVar(&parentBackupDirs, "parent-backup-dirs", "", "parent directories on each host to internally store the backup of the coordinator data directory and user defined coordinator tablespaces."+
		"Defaults to the parent directory of each primary data directory on each primary host."+
		"To specify a single directory across all hosts set a single directory such as /dir."+
		"To specify different directories for each host use the form \"host1:/dir1,host2:/dir2,host3:/dir3\" where the first host must be the coordinator.")

	return addHelpToCommand(cmd, ExecuteHelp)
}
