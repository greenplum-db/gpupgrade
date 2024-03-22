// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"os"
	"os/exec"
	"path"
	"strconv"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/greenplum/connection"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
	"github.com/spf13/cobra"
)

func check() *cobra.Command {
	var sourceGPHome string
	var targetGPHome string
	var sourcePort int

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Executes a subset of pg_upgrade checks for upgrade from GPDB6 to GPDB7",
		Long:  CheckHelp,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			// Get connection to db to get cluster info
			db, err := connection.Bootstrap(idl.ClusterDestination_source, sourceGPHome, sourcePort)
			if err != nil {
				return err
			}
			defer func() {
				if cErr := db.Close(); cErr != nil {
					err = errorlist.Append(err, cErr)
				}
			}()

			source, err := greenplum.ClusterFromDB(db, sourceGPHome, idl.ClusterDestination_source)
			if err != nil {
				return err
			}

			logDir, err := utils.GetLogDir()
			if err != nil {
				return err
			}

			pgUpgradeArgs := []string{
				"-c",
				"--continue-check-on-fatal",
				"--retain",
				"--output-dir", logDir,
				"-d", source.CoordinatorDataDir(),
				// TODO: For reasons currently unknown, target DataDir is
				// needed for --skip-target-check to not fail out.
				"-D", source.CoordinatorDataDir(),
				"-b", path.Join(sourceGPHome, "bin"),
				"-p", strconv.Itoa(sourcePort),
				"--skip-target-check",
				"--check-not-in-place",
			}

			pgUpgradeBinary := path.Join(targetGPHome, "bin", "pg_upgrade")
			command := exec.Command(pgUpgradeBinary, pgUpgradeArgs...)
			command.Stdout = os.Stdout
			command.Stderr = os.Stderr

			pgUpgradeErr := command.Run()
			if err != nil {
				return pgUpgradeErr
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&sourceGPHome, "source-gphome", "/usr/local/gpdb6", "path for the source Greenplum installation")
	cmd.Flags().StringVar(&targetGPHome, "target-gphome", "/usr/local/gpdb7", "path for the target Greenplum installation")
	cmd.Flags().IntVar(&sourcePort, "source-master-port", 5432, "master port for source gpdb cluster")

	return addHelpToCommand(cmd, CheckHelp)
}
