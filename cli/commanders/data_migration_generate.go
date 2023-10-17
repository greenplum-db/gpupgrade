// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commanders

import (
	"bufio"
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/greenplum/connection"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func GenerateDataMigrationScripts(streams step.OutStreams, nonInteractive bool, gphome string, port int, seedDir string, outputDir string, outputDirFS fs.FS) error {
	version, err := greenplum.Version(gphome)
	if err != nil {
		return err
	}

	switch {
	case version.Major == 5:
		seedDir = filepath.Join(seedDir, "5-to-6-seed-scripts")
	case version.Major == 6:
		seedDir = filepath.Join(seedDir, "6-to-7-seed-scripts")
	case version.Major == 7:
		// seedDir = filepath.Join(seedDir, "7-to-8-seed-scripts")
		return nil // TODO: Remove once there are 7 > 8 data migration scripts
	default:
		return fmt.Errorf("failed to find seed scripts for Greenplum version %s under %q", version, seedDir)
	}

	db, err := bootstrapConnectionFunc(idl.ClusterDestination_source, gphome, port)
	if err != nil {
		return err
	}
	defer func() {
		if cErr := db.Close(); cErr != nil {
			err = errorlist.Append(err, cErr)
		}
	}()

	err = utils.System.MkdirAll(outputDir, 0700)
	if err != nil {
		return err
	}

	err = ArchiveDataMigrationScriptsPrompt(streams, nonInteractive, utils.StdinReader, outputDirFS, outputDir)
	if err != nil {
		if errors.Is(err, step.Skip) {
			return nil
		}

		return err
	}

	databases, err := GetDatabases(db, utils.System.DirFS(seedDir))
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(streams.Stdout(), "\nGenerating data migration scripts for %d databases...\n", len(databases))
	if err != nil {
		return err
	}

	progressBar := mpb.New()
	var wg sync.WaitGroup
	errChan := make(chan error, len(databases))

	for _, database := range databases {
		wg.Add(1)
		bar := progressBar.New(int64(database.NumSeedScripts),
			mpb.NopStyle(),
			mpb.PrependDecorators(decor.Name("  "+database.Datname, decor.WCSyncSpaceR)),
			mpb.AppendDecorators(decor.NewPercentage("%d")))

		go func(streams step.OutStreams, database DatabaseInfo, gphome string, port int, seedDir string, outputDir string, bar *mpb.Bar) {
			defer wg.Done()

			err = GenerateScriptsPerDatabase(streams, database, gphome, port, seedDir, outputDir, bar)
			if err != nil {
				errChan <- err
				bar.Abort(false)
				return
			}

		}(streams, database, gphome, port, seedDir, outputDir, bar)
	}

	progressBar.Wait()
	wg.Wait()
	close(errChan)

	var errs error
	for e := range errChan {
		errs = errorlist.Append(errs, e)
	}

	if errs != nil {
		return errs
	}

	logDir, err := utils.GetLogDir()
	if err != nil {
		return err
	}

	fmt.Printf("\nGenerated scripts:%s\nLogs: %s\n\n", utils.Bold.Sprint(filepath.Join(outputDir, "current")), utils.Bold.Sprint(logDir))

	return nil
}

var bootstrapConnectionFunc = connection.Bootstrap

// XXX: for internal testing only
func SetBootstrapConnectionFunction(connectionFunc func(destination idl.ClusterDestination, gphome string, port int) (*sql.DB, error)) {
	bootstrapConnectionFunc = connectionFunc
}

// XXX: for internal testing only
func ResetBootstrapConnectionFunction() {
	bootstrapConnectionFunc = connection.Bootstrap
}

func ArchiveDataMigrationScriptsPrompt(streams step.OutStreams, nonInteractive bool, reader *bufio.Reader, outputDirFS fs.FS, outputDir string) error {
	outputDirEntries, err := utils.System.ReadDirFS(outputDirFS, ".")
	if err != nil {
		return err
	}

	currentDir := filepath.Join(outputDir, "current")
	currentDirExists := false
	var currentDirModTime time.Time
	for _, entry := range outputDirEntries {
		if entry.IsDir() && entry.Name() == "current" {
			currentDirExists = true
			info, eErr := entry.Info()
			if eErr != nil {
				return eErr
			}

			currentDirModTime = info.ModTime()
		}
	}

	if !currentDirExists {
		return nil
	}

	for {
		fmt.Println()
		fmt.Printf(`Previously generated data migration scripts found from
%s located in
%s

Archive and re-generate the data migration scripts if potentially 
new problematic objects have been added since the scripts were 
first generated. If unsure its safe to archive and re-generate 
the scripts.

The generator takes a "snapshot" of the current source cluster
to generate the scripts. If new "problematic" objects are added 
after the generator was run, then the previously generated 
scripts are outdated. The generator will need to be re-run 
to detect the newly added objects.`, currentDirModTime.Format(time.RFC1123Z), utils.Bold.Sprint(currentDir))

		input := "a"
		if !nonInteractive {
			fmt.Println()
			fmt.Printf(`
  [a]rchive and re-generate scripts
  [c]ontinue using previously generated scripts
  [q]uit

Select: `)

			rawInput, rErr := reader.ReadString('\n')
			if rErr != nil {
				return rErr
			}

			input = strings.ToLower(strings.TrimSpace(rawInput))
		}

		switch input {
		case "a":
			archiveDir := filepath.Join(outputDir, "archive", currentDirModTime.Format("20060102T1504"))
			exist, pErr := upgrade.PathExist(archiveDir)
			if pErr != nil {
				return pErr
			}

			if exist {
				log.Printf("Skip archiving data migration scripts as it already exists in %s\n", utils.Bold.Sprint(archiveDir))
				return step.Skip
			}

			fmt.Printf("\nArchiving previously generated scripts under\n%s\n\n", utils.Bold.Sprint(archiveDir))
			err = utils.System.MkdirAll(filepath.Dir(archiveDir), 0700)
			if err != nil {
				return fmt.Errorf("make directory: %w", err)
			}

			err = utils.Move(currentDir, archiveDir)
			if err != nil {
				return fmt.Errorf("move directory: %w", err)
			}

			return nil
		case "c":
			fmt.Printf("\nContinuing with previously generated data migration scripts in\n%s\n", utils.Bold.Sprint(currentDir))
			return step.Skip
		case "q":
			fmt.Print("\nQuitting...")
			return step.Quit
		default:
			continue
		}
	}
}

func GenerateScriptsPerDatabase(streams step.OutStreams, database DatabaseInfo, gphome string, port int, seedDir string, outputDir string, bar *mpb.Bar) error {
	output, err := executeSQLCommand(gphome, port, database.Datname, `CREATE LANGUAGE plpythonu;`)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return err
	}

	log.Print(string(output))

	// Create a schema to use while generating the scripts. However, the generated scripts cannot depend on this
	// schema as its dropped at the end of the generation process. If necessary, the generated scripts can use their
	// own temporary schema.
	output, err = executeSQLCommand(gphome, port, database.Datname, `DROP SCHEMA IF EXISTS __gpupgrade_tmp_generator CASCADE; CREATE SCHEMA __gpupgrade_tmp_generator;`)
	if err != nil {
		return err
	}

	log.Print(string(output))

	output, err = ApplySQLFile(gphome, port, database.Datname, filepath.Join(seedDir, "create_find_view_dep_function.sql"))
	if err != nil {
		return err
	}

	log.Print(string(output))

	var wg sync.WaitGroup
	errChan := make(chan error, len(MigrationScriptPhases))

	for _, phase := range MigrationScriptPhases {
		wg.Add(1)
		_, fErr := fmt.Fprintf(streams.Stdout(), "  Generating %q scripts for %s\n", phase, database.Datname)
		if fErr != nil {
			return fErr
		}

		go func(phase idl.Step, database DatabaseInfo, gphome string, port int, seedDir string, outputDir string, bar *mpb.Bar) {
			defer wg.Done()

			err = GenerateScriptsPerPhase(phase, database, gphome, port, seedDir, utils.System.DirFS(seedDir), outputDir, bar)
			if err != nil {
				errChan <- err
				return
			}
		}(phase, database, gphome, port, seedDir, outputDir, bar)
	}

	wg.Wait()
	close(errChan)

	var errs error
	for e := range errChan {
		errs = errorlist.Append(errs, e)
	}

	if errs != nil {
		return errs
	}

	output, err = executeSQLCommand(gphome, port, database.Datname, `DROP TABLE IF EXISTS __gpupgrade_tmp_generator.__temp_views_list; DROP SCHEMA IF EXISTS __gpupgrade_tmp_generator CASCADE;`)
	if err != nil {
		return err
	}

	log.Println(string(output))
	return nil
}

func isGlobalScript(script string, database string) bool {
	// Generate one global script for the postgres database rather than all databases.
	return database != "postgres" && (script == "gen_alter_gphdfs_roles.sql" || script == "generate_cluster_stats.sh")
}

func GenerateScriptsPerPhase(phase idl.Step, database DatabaseInfo, gphome string, port int, seedDir string, seedDirFS fs.FS, outputDir string, bar *mpb.Bar) error {
	scriptDirs, err := fs.ReadDir(seedDirFS, phase.String())
	if err != nil {
		return err
	}

	if len(scriptDirs) == 0 {
		return xerrors.Errorf("Failed to generate data migration script. No seed files found in %q.", seedDir)
	}

	for _, scriptDir := range scriptDirs {
		scripts, rErr := utils.System.ReadDirFS(seedDirFS, filepath.Join(phase.String(), scriptDir.Name()))
		if rErr != nil {
			return rErr
		}

		for _, script := range scripts {
			if isGlobalScript(script.Name(), database.Datname) {
				continue
			}

			var scriptOutput []byte
			if strings.HasSuffix(script.Name(), ".sql") {
				scriptOutput, err = ApplySQLFile(gphome, port, database.Datname, filepath.Join(seedDir, phase.String(), scriptDir.Name(), script.Name()),
					"-v", "ON_ERROR_STOP=1", "--no-align", "--tuples-only")
				if err != nil {
					return err
				}
			}

			if strings.HasSuffix(script.Name(), ".sh") || strings.HasSuffix(script.Name(), ".bash") {
				scriptOutput, err = executeBashFile(gphome, port, filepath.Join(seedDir, phase.String(), scriptDir.Name(), script.Name()), database.Datname)
				if err != nil {
					return err
				}
			}

			if len(scriptOutput) == 0 {
				// Increment bar even when there is no generated script written since the bar is tied to seed scripts executed rather than written.
				bar.Increment()
				continue
			}

			var contents bytes.Buffer
			contents.WriteString(`\c ` + database.QuotedDatname + "\n")

			headerOutput, fErr := utils.System.ReadFileFS(seedDirFS, filepath.Join(phase.String(), scriptDir.Name(), strings.TrimSuffix(script.Name(), path.Ext(script.Name()))+".header"))
			if fErr != nil && !errors.Is(fErr, fs.ErrNotExist) {
				return fErr
			}

			contents.Write(headerOutput)
			contents.Write(scriptOutput)

			outputPath := filepath.Join(outputDir, "current", phase.String(), scriptDir.Name())
			mErr := utils.System.MkdirAll(outputPath, 0700)
			if mErr != nil {
				return mErr
			}

			outputFile := "migration_" + database.QuotedDatname + "_" + strings.TrimSuffix(script.Name(), filepath.Ext(script.Name())) + ".sql"
			wErr := utils.System.WriteFile(filepath.Join(outputPath, outputFile), contents.Bytes(), 0644)
			if wErr != nil {
				return wErr
			}

			bar.Increment()
		}
	}

	return nil
}

type DatabaseInfo struct {
	Datname        string
	QuotedDatname  string
	NumSeedScripts int
}

func GetDatabases(db *sql.DB, seedDirFS fs.FS) ([]DatabaseInfo, error) {
	rows, err := db.Query(`SELECT datname, quote_ident(datname) AS quoted_datname FROM pg_database WHERE datname != 'template0';`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var databases []DatabaseInfo
	for rows.Next() {
		var database DatabaseInfo
		err = rows.Scan(&database.Datname, &database.QuotedDatname)
		if err != nil {
			return nil, xerrors.Errorf("pg_database: %w", err)
		}

		numSeedScripts, cErr := countSeedScripts(database.Datname, seedDirFS)
		if cErr != nil {
			return nil, cErr
		}

		database.NumSeedScripts = numSeedScripts

		databases = append(databases, database)
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return databases, nil
}

func countSeedScripts(database string, seedDirFS fs.FS) (int, error) {
	var numSeedScripts int

	phasesEntries, err := utils.System.ReadDirFS(seedDirFS, ".")
	if err != nil {
		return 0, err
	}

	for _, phaseEntry := range phasesEntries {
		if !phaseEntry.IsDir() || !isPhase(phaseEntry.Name()) {
			continue
		}

		seedScriptDirs, rErr := fs.ReadDir(seedDirFS, phaseEntry.Name())
		if rErr != nil {
			return 0, rErr
		}

		for _, seedScriptDir := range seedScriptDirs {
			seedScripts, fErr := utils.System.ReadDirFS(seedDirFS, filepath.Join(phaseEntry.Name(), seedScriptDir.Name()))
			if fErr != nil {
				return 0, fErr
			}

			for _, seedScript := range seedScripts {
				if isGlobalScript(seedScript.Name(), database) {
					continue
				}

				numSeedScripts += 1
			}
		}
	}

	return numSeedScripts, nil
}

func isPhase(input string) bool {
	for _, phase := range MigrationScriptPhases {
		if input == phase.String() {
			return true
		}
	}

	return false
}
