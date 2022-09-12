// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commanders

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

func ExecuteDataMigrationScripts(nonInteractive bool, gphome string, port int, currentScriptDirFS fs.FS, currentScriptDir string, phase idl.Step) error {
	_, err := currentScriptDirFS.Open(phase.String())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			fmt.Printf("No %q data migration scripts to execute in %q.\n", phase, filepath.Join(currentScriptDir, phase.String()))
			return nil
		}

		return err
	}

	fmt.Printf("Before executing the %q data migration scripts you can view them in %s\n", phase, filepath.Join(currentScriptDir, phase.String()))

	scriptDirsToRun, err := ExecuteDataMigrationScriptsPrompt(nonInteractive, bufio.NewReader(os.Stdin), currentScriptDir, currentScriptDirFS, phase)
	if err != nil {
		if errors.Is(err, step.Skip) {
			return nil
		}

		return err
	}

	bar := progressbar.NewOptions(len(scriptDirsToRun), progressbar.OptionFullWidth(), progressbar.OptionShowCount(),
		progressbar.OptionClearOnFinish(), progressbar.OptionSetPredictTime(true))

	for _, scriptDir := range scriptDirsToRun {
		_ = bar.Add(1)
		bar.Describe(fmt.Sprintf("  %s...", filepath.Base(scriptDir)))

		output, err := ExecuteDataMigrationScriptSubDir(gphome, port, utils.System.DirFS(scriptDir), scriptDir)
		if err != nil {
			return err
		}

		log.Println(string(output))

		outputPath := filepath.Join(currentScriptDir, phase.String()+".log")
		err = utils.System.WriteFile(outputPath, output, 0644)
		if err != nil {
			return err
		}

		if phase == idl.Step_stats {
			fmt.Printf("To receive an upgrade time estimate send the output of the executed stats scripts in %s\n", outputPath)
		}
	}

	fmt.Printf("\nDone executing %q data migration scripts.\n", phase)

	logDir, err := utils.GetLogDir()
	if err != nil {
		return err
	}

	fmt.Printf("Logs located in %q and %q\n", logDir, currentScriptDir)
	return nil
}

func ExecuteDataMigrationScriptSubDir(gphome string, port int, scriptDirFS fs.FS, scriptDir string) ([]byte, error) {
	entries, err := utils.System.ReadDirFS(scriptDirFS, ".")
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return nil, xerrors.Errorf("Failed to execute data migration script. No SQL files found in %q.", scriptDir)
	}

	var outputs []byte
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".sql" {
			continue
		}

		// FIXME: Disabled ON_ERROR_STOP due to incompatibilities of deprecated objects on 6->6 upgrade that will cause
		//  scripts to fail.
		output, err := executeSQLFile(gphome, port, "postgres", filepath.Join(scriptDir, entry.Name()), "-v", "ON_ERROR_STOP=0", "--echo-queries")
		if err != nil {
			return nil, err
		}

		outputs = append(outputs, output...)
	}

	return outputs, nil
}

func ExecuteDataMigrationScriptsPrompt(nonInteractive bool, reader *bufio.Reader, currentScriptDir string, currentScriptDirFS fs.FS, phase idl.Step) ([]string, error) {
	for {
		var input = "a"
		if !nonInteractive {
			fmt.Printf("\nWould you like execute: [a]ll, [s]ome, or [n]one of the %q data migration scripts? Or [q]uit?\nSelect: ", phase)
			rawinput, err := reader.ReadString('\n')
			if err != nil {
				return nil, err
			}

			input = rawinput
		}

		input = strings.ToLower(strings.TrimSpace(input))
		switch input {
		case "a":
			fmt.Printf("\nExecuting 'all' of the %q data migration scripts.\n", phase)
			entries, err := utils.System.ReadDirFS(currentScriptDirFS, phase.String())
			if err != nil {
				return nil, err
			}

			var scriptDirs []string
			for _, entry := range entries {
				scriptDirs = append(scriptDirs, filepath.Join(currentScriptDir, phase.String(), entry.Name()))
			}

			return scriptDirs, nil
		case "s":
			fmt.Printf("\nSelecting 'some' of the %s data migration scripts.\n", phase)
			scriptDirs, err := SelectDataMigrationScriptsPrompt(bufio.NewReader(os.Stdin), currentScriptDir, currentScriptDirFS, phase)
			if err != nil {
				return nil, err
			}
			return scriptDirs, nil
		case "n":
			fmt.Printf("\nProceeding with 'none' of the %s data migration scripts.\n", phase)
			return nil, step.Skip
		case "q":
			fmt.Print("\nQuiting...")
			return nil, step.UserCanceled
		default:
			continue
		}
	}
}

func SelectDataMigrationScriptsPrompt(reader *bufio.Reader, currentScriptDir string, currentScriptDirFS fs.FS, phase idl.Step) ([]string, error) {
	entries, err := utils.System.ReadDirFS(currentScriptDirFS, phase.String())
	if err != nil {
		return nil, err
	}

	var allScripts Scripts
	for i, script := range entries {
		allScripts = append(allScripts, Script{Num: uint64(i), Name: script.Name()})
	}

	for {
		fmt.Printf("Select which %q data migration scripts to execute separated by commas. Or [q]uit?\n\n%s\nSelect: ", phase, allScripts)
		input, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		selectedScriptDirs, err := ParseSelection(input, allScripts)
		if err != nil {
			if errors.Is(err, step.UserCanceled) {
				fmt.Println()
				fmt.Print("Quiting...")
				return nil, err
			}

			fmt.Println()
			fmt.Println(err)
			continue
		}

		fmt.Printf("\nYou selected scripts:\n\n%s\n", selectedScriptDirs)
		fmt.Printf("[c]ontinue, [e]dit selection, or [q]uit.\nSelect: ")
		input, err = reader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		input = strings.ToLower(strings.TrimSpace(input))
		switch input {
		case "c":
			fmt.Printf("\nExecuting the %q data migration scripts:\n\n%s\n", phase, selectedScriptDirs)

			var scriptDirs []string
			for _, dir := range selectedScriptDirs.Names() {
				scriptDirs = append(scriptDirs, filepath.Join(currentScriptDir, phase.String(), dir))
			}

			return scriptDirs, nil
		case "e":
			continue
		case "q":
			fmt.Print("\nQuiting...")
			return nil, step.UserCanceled
		default:
			continue
		}
	}
}

func ParseSelection(input string, allScripts Scripts) (Scripts, error) {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return nil, fmt.Errorf("Expected a number or numbers separated by commas.")
	}

	if input == "q" {
		return nil, step.UserCanceled
	}

	selections := strings.Split(input, ",")

	var selectedScripts Scripts
	for _, selection := range selections {
		i, err := strconv.ParseUint(strings.TrimSpace(selection), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("Invalid selection. Found %q expected a number or numbers separated by commas.", selection)
		}

		selectedScripts = append(selectedScripts, allScripts.Find(i))
	}

	return selectedScripts, nil
}
