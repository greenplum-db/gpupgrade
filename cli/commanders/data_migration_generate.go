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
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

var CreateConnectionFunc = CreateConnection

func GenerateDataMigrationScripts(nonInteractive bool, gphome string, port int, seedDir string, outputDir string, outputDirFS fs.FS) error {
	version, err := greenplum.Version(gphome)
	if err != nil {
		return err
	}

	switch {
	case version.Major == 5:
		seedDir = filepath.Join(seedDir, "5-to-6-seed-scripts")
	case version.Major == 6:
		seedDir = filepath.Join(seedDir, "6-to-7-seed-scripts")
	//case version.Major == 7:
	//	seedDir = filepath.Join(seedDir, "7-to-8-seed-scripts") // Need to add 7-8 seed scripts for 7-to-7 jobs.
	default:
		return fmt.Errorf("failed to find seed scripts for Greenplum version %s under %q", version, seedDir)
	}

	db, err := CreateConnectionFunc(port)
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

	err = ArchiveDataMigrationScriptsPrompt(nonInteractive, bufio.NewReader(os.Stdin), outputDirFS, outputDir)
	if err != nil {
		if errors.Is(err, step.Skip) {
			return nil
		}

		return err
	}

	databases, err := GetDatabases(db)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(databases))
	fmt.Printf("\nGenerating data migration scripts for %d databases...\n", len(databases))

	for _, database := range databases {
		wg.Add(1)

		go func(database DatabaseName, gphome string, port int, seedDir string, outputDir string) {
			defer wg.Done()

			err = GenerateScriptsPerDatabase(database, gphome, port, seedDir, outputDir)
			if err != nil {
				errChan <- err
				return
			}
		}(database, gphome, port, seedDir, outputDir)
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

	logDir, err := utils.GetLogDir()
	if err != nil {
		return err
	}

	fmt.Printf(`Generated scripts:
%s

Logs:
%s
`, utils.Bold.Sprint(filepath.Join(outputDir, "current")), utils.Bold.Sprint(logDir))

	return nil
}

func ArchiveDataMigrationScriptsPrompt(nonInteractive bool, reader *bufio.Reader, outputDirFS fs.FS, outputDir string) error {
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
			info, err := entry.Info()
			if err != nil {
				return err
			}

			currentDirModTime = info.ModTime()
		}
	}

	if !currentDirExists {
		return nil
	}

	for {
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
			rawInput, err := reader.ReadString('\n')
			if err != nil {
				return err
			}

			input = strings.ToLower(strings.TrimSpace(rawInput))
		}

		switch input {
		case "a":
			archiveDir := filepath.Join(outputDir, "archive", currentDirModTime.Format("2006-01-02T15:04"))
			exist, err := upgrade.PathExist(archiveDir)
			if err != nil {
				return err
			}

			if exist {
				log.Printf("Skip archiving data migration scripts as it already exists in %s\n", utils.Bold.Sprint(archiveDir))
				return step.Skip
			}

			fmt.Printf("\nArchiving previously generated scripts under\n%s\n", utils.Bold.Sprint(archiveDir))
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
			return step.UserCanceled
		default:
			continue
		}
	}
}

func GenerateScriptsPerDatabase(database DatabaseName, gphome string, port int, seedDir string, outputDir string) error {
	output, err := executeSQLCommand(gphome, port, database.Datname, `CREATE LANGUAGE plpythonu;`)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return err
	}

	log.Println(string(output))

	// Create a schema to use while generating the scripts. However, the generated scripts cannot depend on this
	// schema as its dropped at the end of the generation process. If necessary, the generated scripts can use their
	// own temporary schema.
	output, err = executeSQLCommand(gphome, port, database.Datname, `DROP SCHEMA IF EXISTS __gpupgrade_tmp_generator CASCADE; CREATE SCHEMA __gpupgrade_tmp_generator;`)
	if err != nil {
		return err
	}

	log.Println(string(output))

	output, err = applySQLFile(gphome, port, database.Datname, filepath.Join(seedDir, "create_find_view_dep_function.sql"))
	if err != nil {
		return err
	}

	log.Println(string(output))

	var wg sync.WaitGroup
	errChan := make(chan error, len(MigrationScriptPhases))

	for _, phase := range MigrationScriptPhases {
		wg.Add(1)
		fmt.Printf("  Generating %q scripts for %s...\n", phase, database.Datname)

		go func(phase idl.Step, database DatabaseName, gphome string, port int, seedDir string, outputDir string) {
			defer wg.Done()

			err = GenerateScriptsPerPhase(phase, database, gphome, port, seedDir, utils.System.DirFS(seedDir), outputDir)
			if err != nil {
				errChan <- err
				return
			}
		}(phase, database, gphome, port, seedDir, outputDir)
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
	fmt.Println()
	return nil
}

// Generate one global script for the postgres database rather than all databases.
func isGlobalScript(scriptDir string, database string) bool {
	return database != "postgres" && (scriptDir == "gphdfs_user_roles" || scriptDir == "cluster_stats")
}

func GenerateScriptsPerPhase(phase idl.Step, database DatabaseName, gphome string, port int, seedDir string, seedDirFS fs.FS, outputDir string) error {
	scriptDirs, err := fs.ReadDir(seedDirFS, phase.String())
	if err != nil {
		return err
	}

	if len(scriptDirs) == 0 {
		return xerrors.Errorf("Failed to generate data migration script. No seed files found in %q.", seedDir)
	}

	bar := progressbar.NewOptions(len(scriptDirs), progressbar.OptionFullWidth(), progressbar.OptionShowCount(),
		progressbar.OptionClearOnFinish(), progressbar.OptionSetPredictTime(true))

	for _, scriptDir := range scriptDirs {
		if isGlobalScript(scriptDir.Name(), database.Datname) {
			continue
		}

		_ = bar.Add(1)
		bar.Describe(fmt.Sprintf("  %s...", scriptDir.Name()))

		scripts, err := utils.System.ReadDirFS(seedDirFS, filepath.Join(phase.String(), scriptDir.Name()))
		if err != nil {
			return err
		}

		for _, script := range scripts {
			var scriptOutput []byte
			if strings.HasSuffix(script.Name(), ".sql") {
				scriptOutput, err = applySQLFile(gphome, port, database.Datname, filepath.Join(seedDir, phase.String(), scriptDir.Name(), script.Name()),
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
				continue
			}

			var contents bytes.Buffer
			contents.WriteString(`\c ` + database.QuotedDatname + "\n")

			headerOutput, err := utils.System.ReadFileFS(seedDirFS, filepath.Join(phase.String(), scriptDir.Name(), strings.TrimSuffix(script.Name(), path.Ext(script.Name()))+".header"))
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}

			contents.Write(headerOutput)
			contents.Write(scriptOutput)

			outputPath := filepath.Join(outputDir, "current", phase.String(), scriptDir.Name())
			err = utils.System.MkdirAll(outputPath, 0700)
			if err != nil {
				return err
			}

			outputFile := "migration_" + database.QuotedDatname + "_" + strings.TrimSuffix(script.Name(), filepath.Ext(script.Name())) + ".sql"
			err = utils.System.WriteFile(filepath.Join(outputPath, outputFile), contents.Bytes(), 0644)
			if err != nil {
				return err
			}
		}

		err = bar.Clear()
		if err != nil {
			return err
		}
	}

	return nil
}

func CreateConnection(port int) (*sql.DB, error) {
	source, err := greenplum.NewCluster([]greenplum.SegConfig{})
	if err != nil {
		return nil, err
	}

	source.Destination = idl.ClusterDestination_source
	conn := source.Connection([]greenplum.Option{greenplum.Port(port)}...)

	db, err := sql.Open("pgx", conn)
	if err != nil {
		return nil, err
	}

	return db, nil
}

type DatabaseName struct {
	Datname       string
	QuotedDatname string
}

func GetDatabases(db *sql.DB) ([]DatabaseName, error) {
	rows, err := db.Query(`SELECT datname, quote_ident(datname) AS quoted_datname FROM pg_database WHERE datname != 'template0';`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var databases []DatabaseName
	for rows.Next() {
		var database DatabaseName
		err := rows.Scan(&database.Datname, &database.QuotedDatname)
		if err != nil {
			return nil, xerrors.Errorf("pg_database: %w", err)
		}

		databases = append(databases, database)
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return databases, nil
}
