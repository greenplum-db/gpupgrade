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

	"github.com/greenplum-db/gpupgrade/cli/clistep"
	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/config"
	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

func execute() *cobra.Command {
	var verbose bool
	var pgUpgradeVerbose bool
	var skipPgUpgradeChecks bool
	var nonInteractive bool
	var parentBackupDirs string

	cmd := &cobra.Command{
		Use:   "execute",
		Short: "executes the upgrade",
		Long:  ExecuteHelp,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			cmd.SilenceUsage = true
			var response *idl.ExecuteResponse

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

			st, err := clistep.Begin(idl.Step_execute, verbose, nonInteractive, confirmationText)
			if err != nil {
				if errors.Is(err, step.Quit) {
					// If user cancels don't return an error to main to avoid
					// printing "Error:".
					return nil
				}
				return err
			}

			intermediate := &greenplum.Cluster{}
			st.RunHubSubstep(func(streams step.OutStreams) error {
				client, err := connectToHub()
				if err != nil {
					return err
				}

				request := &idl.ExecuteRequest{
					PgUpgradeVerbose:    pgUpgradeVerbose,
					SkipPgUpgradeChecks: skipPgUpgradeChecks,
					ParentBackupDirs:    parentBackupDirs,
				}
				response, err = commanders.Execute(client, request, verbose)
				if err != nil {
					return err
				}

				intermediate, err = greenplum.DecodeCluster(response.GetIntermediate())
				if err != nil {
					return err
				}

				return nil
			})

			return st.Complete(fmt.Sprintf(ExecuteCompletedText,
				filepath.Join(intermediate.GPHome, "greenplum_path.sh"),
				intermediate.CoordinatorDataDir(),
				intermediate.CoordinatorPort()))
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print the output stream from all substeps")
	cmd.Flags().BoolVar(&pgUpgradeVerbose, "pg-upgrade-verbose", false, "execute pg_upgrade with --verbose")
	cmd.Flags().BoolVar(&skipPgUpgradeChecks, "skip-pg-upgrade-checks", false, "skips pg_upgrade checks")
	cmd.Flags().MarkHidden("skip-pg-upgrade-checks") //nolint
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "do not prompt for confirmation to proceed")
	cmd.Flags().MarkHidden("non-interactive") //nolint
	cmd.Flags().StringVar(&parentBackupDirs, "parent-backup-dirs", "", "parent directories on each host to internally store the backup of the coordinator data directory and user defined coordinator tablespaces."+
		"Defaults to the parent directory of each primary data directory on each primary host."+
		"To specify a single directory across all hosts set a single directory such as /dir."+
		"To specify different directories for each host use the form \"host1:/dir1,host2:/dir2,host3:/dir3\" where the first host must be the coordinator.")

	return addHelpToCommand(cmd, ExecuteHelp)
}
