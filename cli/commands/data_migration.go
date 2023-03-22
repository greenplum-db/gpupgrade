// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func dataMigrationGenerate() *cobra.Command {
	var nonInteractive bool
	var gphome string
	var port int
	var seedDir string
	var outputDir string

	logDir, err := utils.GetLogDir()
	if err != nil {
		panic(err)
	}

	outputDir = filepath.Join(logDir, "data-migration-scripts")

	dataMigrationGenerator := &cobra.Command{
		Use:   "generate",
		Short: "generate data migration SQL scripts",
		Long:  "generate data migration SQL scripts",
		RunE: func(cmd *cobra.Command, args []string) error {
			outputDir = filepath.Clean(outputDir)
			seedDir = filepath.Clean(seedDir)
			return commanders.GenerateDataMigrationScripts(nonInteractive, filepath.Clean(gphome), port, seedDir, outputDir, utils.System.DirFS(outputDir))
		},
	}

	dataMigrationGenerator.Flags().BoolVar(&nonInteractive, "non-interactive", false, "do not prompt to proceed")
	dataMigrationGenerator.Flags().MarkHidden("non-interactive") //nolint
	dataMigrationGenerator.Flags().StringVar(&gphome, "gphome", "", "path to the Greenplum installation")
	dataMigrationGenerator.Flags().IntVar(&port, "port", 0, "master port for Greenplum cluster")
	dataMigrationGenerator.Flags().StringVar(&outputDir, "output-dir", outputDir, "output path to the current generated data migration SQL files. Defaults to $HOME/gpAdminLogs/gpupgrade/data-migration-scripts")
	// seed-dir is a hidden flag used for internal testing.
	dataMigrationGenerator.Flags().StringVar(&seedDir, "seed-dir", utils.GetDataMigrationSeedDir(), "path to the seed scripts")
	dataMigrationGenerator.Flags().MarkHidden("seed-dir") //nolint

	return addHelpToCommand(dataMigrationGenerator, generateHelp)
}

func dataMigrationApply() *cobra.Command {
	var nonInteractive bool
	var gphome string
	var port int
	var inputDir string
	var phase string

	logDir, err := utils.GetLogDir()
	if err != nil {
		panic(err)
	}

	inputDir = filepath.Join(logDir, "data-migration-scripts")

	dataMigrationExecutor := &cobra.Command{
		Use:   "apply",
		Short: "apply data migration SQL scripts",
		Long:  "apply data migration SQL scripts",
		RunE: func(cmd *cobra.Command, args []string) error {
			parsedPhase, err := parsePhase(phase)
			if err != nil {
				return err
			}

			currentDir := filepath.Join(filepath.Clean(inputDir), "current")
			err = commanders.ApplyDataMigrationScripts(nonInteractive, filepath.Clean(gphome), port, logDir, utils.System.DirFS(currentDir), currentDir, parsedPhase)
			if err != nil {
				return err
			}

			return nil
		},
	}

	dataMigrationExecutor.Flags().BoolVar(&nonInteractive, "non-interactive", false, "do not prompt to proceed")
	dataMigrationExecutor.Flags().MarkHidden("non-interactive") //nolint
	dataMigrationExecutor.Flags().StringVar(&gphome, "gphome", "", "path to the Greenplum installation")
	dataMigrationExecutor.Flags().IntVar(&port, "port", 0, "master port for Greenplum cluster")
	dataMigrationExecutor.Flags().StringVar(&inputDir, "input-dir", inputDir, "path to the generated data migration SQL files. Defaults to $HOME/gpAdminLogs/gpupgrade/data-migration-scripts")
	dataMigrationExecutor.Flags().StringVar(&phase, "phase", "", `data migration phase. Either "pre-initialize", "post-finalize", "post-revert", or "stats".`)

	return addHelpToCommand(dataMigrationExecutor, applyHelp)
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
