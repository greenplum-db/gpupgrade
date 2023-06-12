// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
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
	"sync"

	"github.com/fatih/color"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func ApplyDataMigrationScripts(nonInteractive bool, gphome string, port int, logDir string, currentScriptDirFS fs.FS, currentScriptDir string, phase idl.Step) error {
	_, err := currentScriptDirFS.Open(phase.String())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			fmt.Printf("No %q data migration scripts to apply in %s.\n", phase, utils.Bold.Sprint(filepath.Join(currentScriptDir, phase.String())))
			return nil
		}

		return err
	}

	fmt.Printf("Inspect the %q data migration SQL scripts in\n%s\n", phase, utils.Bold.Sprint(filepath.Join(currentScriptDir, phase.String())))

	scriptDirsToRun, err := ApplyDataMigrationScriptsPrompt(nonInteractive, bufio.NewReader(os.Stdin), currentScriptDir, currentScriptDirFS, phase)
	if err != nil {
		if errors.Is(err, step.Skip) {
			return nil
		}

		return err
	}

	outputPath := filepath.Join(logDir, "apply_"+phase.String()+".log")
	file, err := utils.System.OpenFile(outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	defer func() {
		if cErr := file.Close(); cErr != nil {
			err = errorlist.Append(err, cErr)
		}
	}()

	progressBar := mpb.New()
	var wg sync.WaitGroup
	errChan := make(chan error, len(scriptDirsToRun))
	outputChan := make(chan []byte, len(scriptDirsToRun))

	fmt.Printf("\nApplying data migration scripts...\n")
	for _, scriptDir := range scriptDirsToRun {
		wg.Add(1)

		scriptDirEntries, err := utils.System.ReadDirFS(utils.System.DirFS(scriptDir), ".")
		if err != nil {
			return err
		}

		bar := progressBar.New(int64(countScripts(scriptDirEntries)),
			mpb.NopStyle(),
			mpb.PrependDecorators(
				decor.Name("  "+filepath.Base(scriptDir), decor.WCSyncSpaceR),
				decor.CountersNoUnit("  %d/%d scripts applied")))

		go func(gphome string, port int, scriptDir string, bar *mpb.Bar) {
			defer wg.Done()

			output, err := ApplyDataMigrationScriptSubDir(gphome, port, utils.System.DirFS(scriptDir), scriptDir, bar)
			if err != nil {
				errChan <- err
				bar.Abort(false)
				return
			}

			outputChan <- output
		}(gphome, port, scriptDir, bar)
	}

	progressBar.Wait()
	wg.Wait()
	close(errChan)
	close(outputChan)

	var errs error
	for e := range errChan {
		errs = errorlist.Append(errs, e)
	}

	if errs != nil {
		return errs
	}

	for output := range outputChan {
		log.Println(string(output))

		_, err = file.Write(output)
		if err != nil {
			return err
		}
	}

	if phase == idl.Step_stats {
		fmt.Print(color.YellowString("\nTo receive an upgrade time estimate send the stats output:\n%s\n", utils.Bold.Sprint(filepath.Join(logDir, "apply_"+phase.String()+".log"))))
	}

	fmt.Printf(`
Logs:
%s

`, utils.Bold.Sprint(logDir))

	return nil
}

func countScripts(entries []fs.DirEntry) int {
	var numScripts int
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".sql" {
			continue
		}

		numScripts += 1
	}

	return numScripts
}

func ApplyDataMigrationScriptSubDir(gphome string, port int, scriptDirFS fs.FS, scriptDir string, bar *mpb.Bar) ([]byte, error) {
	entries, err := utils.System.ReadDirFS(scriptDirFS, ".")
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return nil, xerrors.Errorf("Failed to apply data migration script. No SQL files found in %q.", scriptDir)
	}

	var outputs []byte
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".sql" {
			continue
		}

		log.Printf("  %s\n", entry.Name())
		output, err := applySQLFile(gphome, port, "postgres", filepath.Join(scriptDir, entry.Name()), "-v", "ON_ERROR_STOP=1", "--echo-queries")
		if err != nil {
			return nil, err
		}

		outputs = append(outputs, output...)

		bar.Increment()
	}

	return outputs, nil
}

func ApplyDataMigrationScriptsPrompt(nonInteractive bool, reader *bufio.Reader, currentScriptDir string, currentScriptDirFS fs.FS, phase idl.Step) ([]string, error) {
	entries, err := utils.System.ReadDirFS(currentScriptDirFS, phase.String())
	if err != nil {
		return nil, err
	}

	var allScripts Scripts
	for i, script := range entries {
		allScripts = append(allScripts, Script{Num: uint64(i), Name: script.Name()})
	}

	fmt.Println()
	fmt.Printf(`Scripts to apply:
%s`, allScripts.Description())

	for {
		var input = "a"
		if !nonInteractive {
			prompt := fmt.Sprintf(`Which %q data migration SQL scripts to apply?
  [n]one
  [s]ome
  [a]ll
  [q]uit

Select: `, phase)

			if phase == idl.Step_initialize {
				warning := color.RedString(utils.Bold.Sprint(`WARNING: Data migration scripts can leave the source cluster in a non-optimal state 
         and can take time to fully revert.`))

				prompt = fmt.Sprintf(`Which %q data migration SQL scripts to apply?

%s

  [n]o scripts.   When running 'before' the upgrade to uncover pg_upgrade --check errors 
                  there is no need to run the data migration SQL scripts.
  [s]ome scripts. Usually run 'before' the upgrade during maintenance windows to run 
                  selected scripts as suggested in the documentation.
  [a]ll scripts.  Usually run 'during' the upgrade within the downtime window.
  [q]uit

Select: `, phase, warning)
			}

			fmt.Print(prompt)
			rawinput, err := reader.ReadString('\n')
			if err != nil {
				return nil, err
			}

			input = strings.ToLower(strings.TrimSpace(rawinput))
		}

		switch input {
		case "a":
			fmt.Printf("\nApplying 'all' of the %q data migration scripts.\n\n", phase)
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
			scriptDirs, err := SelectDataMigrationScriptsPrompt(bufio.NewReader(os.Stdin), currentScriptDir, currentScriptDirFS, phase)
			if err != nil {
				return nil, err
			}
			return scriptDirs, nil
		case "n":
			fmt.Printf("\nProceeding with 'none' of the %s data migration scripts.\n", phase)
			return nil, step.Skip
		case "q":
			fmt.Print("\nQuitting...\n")
			return nil, step.Quit
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
		fmt.Printf("\nSelect scripts to apply separated by commas such as 1, 3. Or [q]uit?\n\n%s\nSelect: ", allScripts)
		input, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		selectedScriptDirs, err := ParseSelection(input, allScripts)
		if err != nil {
			if errors.Is(err, step.Quit) {
				fmt.Println()
				fmt.Print("Quitting...")
				return nil, err
			}

			fmt.Println()
			fmt.Println(err)
			continue
		}

		fmt.Printf("\nSelected:\n\n%s\n", selectedScriptDirs)
		fmt.Printf("[c]ontinue, [e]dit selection, or [q]uit.\nSelect: ")
		input, err = reader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		input = strings.ToLower(strings.TrimSpace(input))
		switch input {
		case "c":
			fmt.Printf("\nApplying the %q data migration scripts:\n\n%s\n", phase, selectedScriptDirs)

			var scriptDirs []string
			for _, dir := range selectedScriptDirs.Names() {
				scriptDirs = append(scriptDirs, filepath.Join(currentScriptDir, phase.String(), dir))
			}

			return scriptDirs, nil
		case "e":
			continue
		case "q":
			fmt.Print("\nQuitting...")
			return nil, step.Quit
		default:
			continue
		}
	}
}

func ParseSelection(input string, allScripts Scripts) (Scripts, error) {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return nil, fmt.Errorf("Expected a number or numbers separated by commas such as 1, 3.")
	}

	if input == "q" {
		return nil, step.Quit
	}

	selections := strings.Split(input, ",")

	var selectedScripts Scripts
	for _, selection := range selections {
		i, err := strconv.ParseUint(strings.TrimSpace(selection), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("Invalid selection. Found %q expected a number or numbers separated by commas such as 1, 3.", selection)
		}

		selectedScripts = append(selectedScripts, allScripts.Find(i))
	}

	return selectedScripts, nil
}
