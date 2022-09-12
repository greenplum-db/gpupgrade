// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func dataMigrationGenerator() *cobra.Command {
	var nonInteractive bool
	var gphome string
	var port int
	var seedDir string
	var outputDir string

	dataMigrationGenerator := &cobra.Command{
		Use:   "generator",
		Short: "data migration SQL scripts generator",
		Long:  "data migration SQL scripts generator",
		RunE: func(cmd *cobra.Command, args []string) error {
			gphome = filepath.Clean(gphome)
			version, err := greenplum.Version(gphome)
			if err != nil {
				return err
			}

			outputDir = filepath.Clean(outputDir)
			seedDir = filepath.Clean(seedDir)
			switch {
			case version.Major == 5:
				seedDir = filepath.Join(seedDir, "5-to-6-seed-scripts")
			case version.Major == 6:
				seedDir = filepath.Join(seedDir, "6-to-7-seed-scripts")
			case version.Major == 7:
				seedDir = filepath.Join(seedDir, "6-to-7-seed-scripts")
			default:
				return fmt.Errorf("failed to find seed scripts for Greenplum version %s under %q", version, seedDir)
			}

			return commanders.GenerateDataMigrationScripts(nonInteractive, gphome, port, seedDir, utils.System.DirFS(seedDir), outputDir, utils.System.DirFS(outputDir))
		},
	}

	defaultGeneratedScriptsDir, err := utils.GetDefaultDataMigrationGeneratedScriptsDir()
	if err != nil {
		return nil
	}

	dataMigrationGenerator.Flags().BoolVar(&nonInteractive, "non-interactive", false, "do not prompt to proceed")
	dataMigrationGenerator.Flags().MarkHidden("non-interactive") //nolint
	dataMigrationGenerator.Flags().StringVar(&gphome, "gphome", "", "path to the Greenplum installation")
	dataMigrationGenerator.Flags().IntVar(&port, "port", 0, "master port for Greenplum cluster")
	dataMigrationGenerator.Flags().StringVar(&outputDir, "output-dir", defaultGeneratedScriptsDir, "output path to the current generated data migration SQL files. Defaults to $HOME/gpAdminLogs/gpupgrade/data-migration-scripts")
	// seed-dir is a hidden flag used for internal testing.
	dataMigrationGenerator.Flags().StringVar(&seedDir, "seed-dir", utils.GetDataMigrationSeedDir(), "path to the seed scripts")
	dataMigrationGenerator.Flags().MarkHidden("seed-dir") //nolint

	return addHelpToCommand(dataMigrationGenerator, generatorHelp)
}

func dataMigrationExecutor() *cobra.Command {
	var nonInteractive bool
	var gphome string
	var port int
	var inputDir string
	var phase string

	dataMigrationExecutor := &cobra.Command{
		Use:   "executor",
		Short: "data migration SQL scripts executor",
		Long:  "data migration SQL scripts executor",
		RunE: func(cmd *cobra.Command, args []string) error {
			parsedPhase, err := parsePhase(phase)
			if err != nil {
				return err
			}

			currentDir := filepath.Join(filepath.Clean(inputDir), "current")
			err = commanders.ExecuteDataMigrationScripts(nonInteractive, filepath.Clean(gphome), port, utils.System.DirFS(currentDir), currentDir, parsedPhase)
			if err != nil {
				return err
			}

			return nil
		},
	}

	defaultGeneratedScriptsDir, err := utils.GetDefaultDataMigrationGeneratedScriptsDir()
	if err != nil {
		return nil
	}

	dataMigrationExecutor.Flags().BoolVar(&nonInteractive, "non-interactive", false, "do not prompt to proceed")
	dataMigrationExecutor.Flags().MarkHidden("non-interactive") //nolint
	dataMigrationExecutor.Flags().StringVar(&gphome, "gphome", "", "path to the Greenplum installation")
	dataMigrationExecutor.Flags().IntVar(&port, "port", 0, "master port for Greenplum cluster")
	dataMigrationExecutor.Flags().StringVar(&inputDir, "input-dir", defaultGeneratedScriptsDir, "path to the generated data migration SQL files. Defaults to $HOME/gpAdminLogs/gpupgrade/data-migration-scripts")
	dataMigrationExecutor.Flags().StringVar(&phase, "phase", "", `data migration phase. Either "pre-initialize", "post-finalize", "post-revert", or "stats".`)

	return addHelpToCommand(dataMigrationExecutor, executorHelp)
}

func parsePhase(input string) (idl.Step, error) {
	inputPhase := idl.Step_value[strings.TrimSpace(input)]

	for _, phase := range commanders.MigrationScriptPhases {
		if idl.Step(inputPhase) == phase {
			return phase, nil
		}
	}

	return idl.Step_unknown_step, fmt.Errorf("Invalid phase %q. Please specify either %s.", input, commanders.MigrationScriptPhases)
}
