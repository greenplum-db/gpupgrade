// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commanders_test

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"
)

func PostgresGPVersion_6_7_1() {
	fmt.Println("postgres (Greenplum Database) 6.7.1 build commit:a21de286045072d8d1df64fa48752b7dfac8c1b7")
}

func init() {
	exectest.RegisterMains(
		PostgresGPVersion_6_7_1,
	)
}

func TestGenerateDataMigrationScripts(t *testing.T) {
	greenplum.SetVersionCommand(exectest.NewCommand(PostgresGPVersion_6_7_1))
	defer greenplum.ResetVersionCommand()

	t.Run("errors when failing to create output directory", func(t *testing.T) {
		expected := os.ErrPermission
		utils.System.MkdirAll = func(path string, perm os.FileMode) error {
			return expected
		}
		defer utils.ResetSystemFunctions()

		err := commanders.GenerateDataMigrationScripts(false, "", 0, "", "", fstest.MapFS{})
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("got error %#v want %#v", err, os.ErrPermission)
		}
	})

	t.Run("returns without error when archiving scripts is skipped", func(t *testing.T) {
		utils.System.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		defer utils.ResetSystemFunctions()

		resetStdin := testutils.SetStdin(t, "c\n")
		defer resetStdin()

		outputDirFS := fstest.MapFS{"current": {Mode: os.ModeDir}}

		err := commanders.GenerateDataMigrationScripts(false, "", 0, "", "", outputDirFS)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("errors when archiving scripts fails", func(t *testing.T) {
		utils.System.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		defer utils.ResetSystemFunctions()

		expected := os.ErrPermission
		utils.System.ReadDirFS = func(fsys fs.FS, name string) ([]fs.DirEntry, error) {
			return nil, expected
		}
		defer utils.ResetSystemFunctions()

		err := commanders.GenerateDataMigrationScripts(false, "", 0, "", "", fstest.MapFS{})
		if !errors.Is(err, expected) {
			t.Errorf("got %v want %v", err, expected)
		}
	})

	t.Run("errors when getting databases fails", func(t *testing.T) {
		utils.System.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		defer utils.ResetSystemFunctions()

		err := commanders.GenerateDataMigrationScripts(false, "", 0, "", "", fstest.MapFS{})
		expected := "invalid port"
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("got %+v, want %+v", err, expected)
		}
	})

	t.Run("does not error when plpythonu is present", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("couldn't create sqlmock: %v", err)
		}
		defer testutils.FinishMock(mock, t)

		commanders.CreateConnectionFunc = func(port int) (*sql.DB, error) {
			return db, nil
		}
		defer func() {
			commanders.CreateConnectionFunc = commanders.CreateConnection
		}()

		outputDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, outputDir)

		testutils.MustCreateDir(t, filepath.Join(outputDir, "current"))

		expectPgDatabaseToReturn(mock).WillReturnRows(sqlmock.NewRows([]string{"datname", "quoted_datname"}).AddRow("postgres", "postgres"))

		numCalls := 0
		commanders.SetPsqlCommand(exectest.NewCommandWithVerifier(commanders.Success, func(utility string, args ...string) {
			numCalls++

			expectedUtility := "/usr/local/gpdb5/bin/psql"
			if utility != expectedUtility {
				t.Errorf("got %q want %q", utility, expectedUtility)
			}

			actualSql := args[7:8]
			if numCalls == 1 {
				expected := []string{"CREATE LANGUAGE plpythonu;"}
				if !reflect.DeepEqual(actualSql, expected) {
					t.Errorf("got sql %q, want %q", actualSql, expected)
				}
			}

			if numCalls == 2 {
				expected := []string{"DROP SCHEMA IF EXISTS __gpupgrade_tmp_generator CASCADE; CREATE SCHEMA __gpupgrade_tmp_generator;"}
				if !reflect.DeepEqual(actualSql, expected) {
					t.Errorf("got sql %q, want %q", actualSql, expected)
				}
			}
		}))
		defer commanders.ResetPsqlCommand()

		commanders.SetPsqlFileCommand(exectest.NewCommand(commanders.Success))
		defer commanders.ResetPsqlFileCommand()

		utils.System.DirFS = func(dir string) fs.FS {
			return fstest.MapFS{
				idl.Step_initialize.String(): {Mode: os.ModeDir},
				filepath.Join(idl.Step_initialize.String(), "unique_primary_foreign_key_constraint"):                                                                {Mode: os.ModeDir},
				filepath.Join(idl.Step_initialize.String(), "unique_primary_foreign_key_constraint", "migration_postgres_gen_drop_constraint_2_primary_unique.sql"): {},
				idl.Step_execute.String(): {Mode: os.ModeDir},
				filepath.Join(idl.Step_execute.String(), "unique_primary_foreign_key_constraint"):                                                                {Mode: os.ModeDir},
				filepath.Join(idl.Step_execute.String(), "unique_primary_foreign_key_constraint", "migration_postgres_gen_drop_constraint_2_primary_unique.sql"): {},
				idl.Step_finalize.String(): {Mode: os.ModeDir},
				filepath.Join(idl.Step_finalize.String(), "unique_primary_foreign_key_constraint"):                                                                {Mode: os.ModeDir},
				filepath.Join(idl.Step_finalize.String(), "unique_primary_foreign_key_constraint", "migration_postgres_gen_drop_constraint_2_primary_unique.sql"): {},
				idl.Step_revert.String(): {Mode: os.ModeDir},
				filepath.Join(idl.Step_revert.String(), "unique_primary_foreign_key_constraint"):                                                                {Mode: os.ModeDir},
				filepath.Join(idl.Step_revert.String(), "unique_primary_foreign_key_constraint", "migration_postgres_gen_drop_constraint_2_primary_unique.sql"): {},
				idl.Step_stats.String(): {Mode: os.ModeDir},
				filepath.Join(idl.Step_stats.String(), "unique_primary_foreign_key_constraint"):                                                                {Mode: os.ModeDir},
				filepath.Join(idl.Step_stats.String(), "unique_primary_foreign_key_constraint", "migration_postgres_gen_drop_constraint_2_primary_unique.sql"): {},
			}
		}
		defer utils.ResetSystemFunctions()

		err = commanders.GenerateDataMigrationScripts(true, "/usr/local/gpdb5", 0, "", outputDir, fstest.MapFS{})
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("errors when creating plpythonu fails with other error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("couldn't create sqlmock: %v", err)
		}
		defer testutils.FinishMock(mock, t)

		commanders.CreateConnectionFunc = func(port int) (*sql.DB, error) {
			return db, nil
		}
		defer func() {
			commanders.CreateConnectionFunc = commanders.CreateConnection
		}()

		expectPgDatabaseToReturn(mock).WillReturnRows(sqlmock.NewRows([]string{"datname", "quoted_datname"}).AddRow("postgres", "postgres"))

		utils.System.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		defer utils.ResetSystemFunctions()

		commanders.SetPsqlCommand(exectest.NewCommand(commanders.FailedMain))
		defer commanders.ResetPsqlCommand()

		err = commanders.GenerateDataMigrationScripts(false, "", 0, "", "", fstest.MapFS{})
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Errorf("got %T, want %T", err, exitError)
		}
	})

	t.Run("errors when create_find_view_dep_function.sql fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("couldn't create sqlmock: %v", err)
		}
		defer testutils.FinishMock(mock, t)

		commanders.CreateConnectionFunc = func(port int) (*sql.DB, error) {
			return db, nil
		}
		defer func() {
			commanders.CreateConnectionFunc = commanders.CreateConnection
		}()

		expectPgDatabaseToReturn(mock).WillReturnRows(sqlmock.NewRows([]string{"datname", "quoted_datname"}).AddRow("postgres", "postgres"))

		utils.System.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		defer utils.ResetSystemFunctions()

		commanders.SetPsqlCommand(exectest.NewCommand(commanders.Success))
		defer commanders.ResetPsqlCommand()

		commanders.SetPsqlFileCommand(exectest.NewCommand(commanders.FailedMain))
		defer commanders.ResetPsqlFileCommand()

		err = commanders.GenerateDataMigrationScripts(false, "", 0, "", "", fstest.MapFS{})
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Errorf("got %T, want %T", err, exitError)
		}
	})

	t.Run("errors when fialing to generate migration script", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("couldn't create sqlmock: %v", err)
		}
		defer testutils.FinishMock(mock, t)

		commanders.CreateConnectionFunc = func(port int) (*sql.DB, error) {
			return db, nil
		}
		defer func() {
			commanders.CreateConnectionFunc = commanders.CreateConnection
		}()

		expectPgDatabaseToReturn(mock).WillReturnRows(sqlmock.NewRows([]string{"datname", "quoted_datname"}).AddRow("postgres", "postgres"))

		utils.System.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		defer utils.ResetSystemFunctions()

		commanders.SetPsqlCommand(exectest.NewCommand(commanders.Success))
		defer commanders.ResetPsqlCommand()

		commanders.SetPsqlFileCommand(exectest.NewCommand(commanders.Success))
		defer commanders.ResetPsqlFileCommand()

		err = commanders.GenerateDataMigrationScripts(false, "", 0, "", "", fstest.MapFS{})
		var expected *os.PathError
		if !errors.As(err, &expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})
}

func TestArchiveDataMigrationScriptsPrompt(t *testing.T) {
	fsys := fstest.MapFS{
		"current": {Mode: os.ModeDir},
	}

	t.Run("errors when failing to read current directory", func(t *testing.T) {
		expected := os.ErrPermission
		utils.System.ReadDirFS = func(fsys fs.FS, name string) ([]fs.DirEntry, error) {
			return nil, expected
		}
		defer utils.ResetSystemFunctions()

		err := commanders.ArchiveDataMigrationScriptsPrompt(false, nil, fsys, "")
		if !errors.Is(err, expected) {
			t.Errorf("got %v want %v", err, expected)
		}
	})

	t.Run("returns if scripts are 'not' already generated and there is nothing to archive", func(t *testing.T) {
		err := commanders.ArchiveDataMigrationScriptsPrompt(false, nil, fstest.MapFS{}, "")
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("errors when failing to read input", func(t *testing.T) {
		reader := bufio.NewReader(strings.NewReader(""))
		err := commanders.ArchiveDataMigrationScriptsPrompt(false, reader, fsys, "")
		expected := io.EOF
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})

	t.Run("archives previously generated scripts when user selects [a]rchive", func(t *testing.T) {
		outputDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, outputDir)

		testutils.MustCreateDir(t, filepath.Join(outputDir, "current"))

		reader := bufio.NewReader(strings.NewReader("a\n"))
		err := commanders.ArchiveDataMigrationScriptsPrompt(false, reader, fsys, outputDir)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		testutils.PathMustExist(t, filepath.Join(outputDir, "archive"))
		testutils.PathMustNotExist(t, filepath.Join(outputDir, "current"))
	})

	t.Run("errors when failing to make archive directory when user selects [a]rchive", func(t *testing.T) {
		expected := os.ErrPermission
		utils.System.MkdirAll = func(path string, perm os.FileMode) error {
			return expected
		}
		defer utils.ResetSystemFunctions()

		reader := bufio.NewReader(strings.NewReader("a\n"))
		err := commanders.ArchiveDataMigrationScriptsPrompt(false, reader, fsys, "")
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("got error %#v want %#v", err, os.ErrPermission)
		}
	})

	t.Run("errors when failing to move current directory to archive directory when user selects [a]rchive", func(t *testing.T) {
		utils.System.MkdirAll = func(path string, perm os.FileMode) error {
			return nil // don't actually create the directory which causes the later move to fail
		}
		defer utils.ResetSystemFunctions()

		reader := bufio.NewReader(strings.NewReader("a\n"))
		err := commanders.ArchiveDataMigrationScriptsPrompt(false, reader, fsys, "")
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Errorf("got %T, want %T", err, exitError)
		}
	})

	t.Run("returns skip error when user selects 'c'ontinue", func(t *testing.T) {
		reader := bufio.NewReader(strings.NewReader("c\n"))
		err := commanders.ArchiveDataMigrationScriptsPrompt(false, reader, fsys, "")
		expected := step.Skip
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})

	t.Run("returns canceled error when user selects 'q'uit", func(t *testing.T) {
		reader := bufio.NewReader(strings.NewReader("q\n"))
		err := commanders.ArchiveDataMigrationScriptsPrompt(false, reader, fsys, "")
		expected := step.UserCanceled
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})

	t.Run("re-prompts when user enters 'b'ad input", func(t *testing.T) {
		d := commanders.BufferStandardDescriptors(t)

		reader := bufio.NewReader(strings.NewReader("b\nq\n"))
		err := commanders.ArchiveDataMigrationScriptsPrompt(false, reader, fsys, "")
		if !errors.Is(err, step.UserCanceled) {
			t.Errorf("got error %#v, want %#v", err, step.UserCanceled)
		}

		stdout, stderr := d.Collect()
		d.Close()
		if len(stderr) != 0 {
			t.Errorf("unexpected stderr %#v", string(stderr))
		}

		actual := string(stdout)
		expected := "[a]rchive and re-generate scripts, [c]ontinue using previously generated scripts, or [q]uit."
		matches := 0
		for _, part := range strings.Split(actual, "\n") {
			if strings.Contains(part, expected) {
				matches++
			}
		}

		if matches != 2 {
			t.Errorf("got %d matches, want 2", matches)
		}
	})
}

func TestGenerateMigrationScript(t *testing.T) {
	phase := idl.Step_initialize
	gphome := "/usr/local/gpdb5"
	port := 123
	database := commanders.DatabaseName{Datname: "postgres", QuotedDatname: "postgres"}
	seedDir := "/usr/local/bin/greenplum/gpupgrade/data-migration-scripts/5-to-6-seed-scripts"
	outputDir := "/home/gpupgrade/data-migration"

	fsys := fstest.MapFS{
		idl.Step_initialize.String():                                                                      {Mode: os.ModeDir},
		filepath.Join(idl.Step_initialize.String(), "gphdfs_user_roles"):                                  {Mode: os.ModeDir},
		filepath.Join(idl.Step_initialize.String(), "gphdfs_user_roles", "gen_alter_gphdfs_roles.header"): {Data: []byte("gphdfs roles header\n")},
		filepath.Join(idl.Step_initialize.String(), "gphdfs_user_roles", "gen_alter_gphdfs_roles.sql"):    {},
	}

	t.Run("errors when failing to read seed directory", func(t *testing.T) {
		err := commanders.GenerateMigrationScript(phase, seedDir, fstest.MapFS{}, outputDir, gphome, port, database)
		var expected *os.PathError
		if !errors.As(err, &expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})

	t.Run("errors when no script directories are found in the seed directory", func(t *testing.T) {
		fsys := fstest.MapFS{
			phase.String(): {Mode: os.ModeDir},
		}

		err := commanders.GenerateMigrationScript(phase, seedDir, fsys, outputDir, gphome, port, database)
		expected := "No seed files found"
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})

	t.Run("only generates one global script rather than multiple scripts per database", func(t *testing.T) {
		commanders.SetPsqlFileCommand(exectest.NewCommand(commanders.SuccessScript))
		defer commanders.ResetPsqlFileCommand()

		utils.System.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		defer utils.ResetSystemFunctions()

		utils.System.ReadFile = func(filename string) ([]byte, error) {
			return nil, nil
		}
		defer utils.ResetSystemFunctions()

		writeGeneratedScriptCalled := false
		utils.System.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writeGeneratedScriptCalled = true

			expected := filepath.Join(outputDir, "current", phase.String(), "gphdfs_user_roles", "migration_postgres_gen_alter_gphdfs_roles.sql")
			if filename != expected {
				t.Errorf("got filename %q, want %q", filename, expected)
			}

			return nil
		}
		defer utils.ResetSystemFunctions()

		err := commanders.GenerateMigrationScript(phase, seedDir, fsys, outputDir, gphome, port, database)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		if !writeGeneratedScriptCalled {
			t.Error("expected writeFile to be called for generated script")
		}
	})

	t.Run("errors when failing to execute SQL script", func(t *testing.T) {
		commanders.SetPsqlFileCommand(exectest.NewCommand(commanders.FailedMain))
		defer commanders.ResetPsqlFileCommand()

		err := commanders.GenerateMigrationScript(phase, seedDir, fsys, outputDir, gphome, port, database)
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Errorf("got %T, want %T", err, exitError)
		}
	})

	t.Run("errors when failing to execute .sh bash script", func(t *testing.T) {
		commanders.SetBashCommand(exectest.NewCommand(commanders.FailedMain))
		defer commanders.ResetBashCommand()

		fsys := fstest.MapFS{
			phase.String(): {Mode: os.ModeDir},
			filepath.Join(phase.String(), "gphdfs_user_roles"):                        {Mode: os.ModeDir},
			filepath.Join(phase.String(), "gphdfs_user_roles", "some_bash_script.sh"): {},
		}

		err := commanders.GenerateMigrationScript(phase, seedDir, fsys, outputDir, gphome, port, database)
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Errorf("got %T, want %T", err, exitError)
		}
	})

	t.Run("errors when failing to execute .bash bash script", func(t *testing.T) {
		commanders.SetBashCommand(exectest.NewCommand(commanders.FailedMain))
		defer commanders.ResetBashCommand()

		fsys := fstest.MapFS{
			phase.String(): {Mode: os.ModeDir},
			filepath.Join(phase.String(), "gphdfs_user_roles"):                          {Mode: os.ModeDir},
			filepath.Join(phase.String(), "gphdfs_user_roles", "some_bash_script.bash"): {},
		}

		err := commanders.GenerateMigrationScript(phase, seedDir, fsys, outputDir, gphome, port, database)
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Errorf("got %T, want %T", err, exitError)
		}
	})

	t.Run("errors when failing to read script directory", func(t *testing.T) {
		expected := os.ErrPermission
		utils.System.ReadDirFS = func(fsys fs.FS, name string) ([]fs.DirEntry, error) {
			return nil, expected
		}
		defer utils.ResetSystemFunctions()

		err := commanders.GenerateMigrationScript(phase, seedDir, fsys, outputDir, gphome, port, database)
		if !errors.Is(err, expected) {
			t.Errorf("got %v want %v", err, expected)
		}
	})

	t.Run("executes sql scripts with correct arguments for the correct database", func(t *testing.T) {
		commanders.SetPsqlFileCommand(exectest.NewCommandWithVerifier(commanders.SuccessScript, func(utility string, args ...string) {
			expectedUtility := "/usr/local/gpdb5/bin/psql"
			if utility != expectedUtility {
				t.Errorf("got %q want %q", utility, expectedUtility)
			}

			additionalArgs := args[:6]
			expected := []string{"-v", "ON_ERROR_STOP=1", "--no-align", "--tuples-only", "--no-psqlrc", "--quiet"}
			if !reflect.DeepEqual(additionalArgs, expected) {
				t.Errorf("got args %q want %q", additionalArgs, expected)
			}

			database := args[7:8]
			expectedDatabase := []string{"postgres"}
			if !reflect.DeepEqual(database, expectedDatabase) {
				t.Errorf("got database %q, want %q", database, expectedDatabase)
			}

			port := args[9:10]
			expectedPort := []string{"123"}
			if !reflect.DeepEqual(port, expectedPort) {
				t.Errorf("got port %q, want %q", port, expectedPort)
			}

			seedScript := args[11:12]
			expectedSeedScript := []string{filepath.Join(seedDir, phase.String(), "unique_primary_foreign_key_constraint", "migration_postgres_gen_drop_constraint_2_primary_unique.sql")}
			if !reflect.DeepEqual(seedScript, expectedSeedScript) {
				t.Errorf("got seed script %q, want %q", seedScript, expectedSeedScript)
			}

		}))
		defer commanders.ResetPsqlFileCommand()

		utils.System.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		defer utils.ResetSystemFunctions()

		writeGeneratedScriptCalled := false
		utils.System.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writeGeneratedScriptCalled = true

			expected := filepath.Join(outputDir, "current", phase.String(), "unique_primary_foreign_key_constraint", "migration_postgres_migration_postgres_gen_drop_constraint_2_primary_unique.sql")
			if filename != expected {
				t.Errorf("got filename %q, want %q", filename, expected)
			}

			expected = "\\c postgres\nsuccessfully executed data migration SQL script"
			actual := string(data)
			if actual != expected {
				t.Errorf("got generated file contents %q, want %q", actual, expected)
			}

			return nil
		}
		defer utils.ResetSystemFunctions()

		fsys := fstest.MapFS{
			phase.String(): {Mode: os.ModeDir},
			filepath.Join(phase.String(), "unique_primary_foreign_key_constraint"):                                                                {Mode: os.ModeDir},
			filepath.Join(phase.String(), "unique_primary_foreign_key_constraint", "migration_postgres_gen_drop_constraint_2_primary_unique.sql"): {},
		}

		err := commanders.GenerateMigrationScript(phase, seedDir, fsys, outputDir, gphome, port, database)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		if !writeGeneratedScriptCalled {
			t.Error("expected writeFile to be called for generated script")
		}
	})

	t.Run("executes bash scripts for the correct database", func(t *testing.T) {
		commanders.SetBashCommand(exectest.NewCommandWithVerifier(commanders.SuccessScript, func(utility string, args ...string) {
			expectedUtility := filepath.Join(seedDir, idl.Step_stats.String(), "database_stats", "generate_database_stats.sh")
			if utility != expectedUtility {
				t.Errorf("got %q want %q", utility, expectedUtility)
			}

			actualArgs := args[0:]
			expected := []string{gphome, strconv.Itoa(port), database.Datname}
			if !reflect.DeepEqual(actualArgs, expected) {
				t.Errorf("got args %q want %q", actualArgs, expected)
			}
		}))
		defer commanders.ResetPsqlCommand()

		utils.System.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		defer utils.ResetSystemFunctions()

		writeGeneratedScriptCalled := false
		utils.System.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writeGeneratedScriptCalled = true

			expected := filepath.Join(outputDir, "current", idl.Step_stats.String(), "database_stats", "migration_postgres_generate_database_stats.sql")
			if filename != expected {
				t.Errorf("got filename %q, want %q", filename, expected)
			}

			expected = "\\c postgres\nsuccessfully executed data migration SQL script"
			actual := string(data)
			if actual != expected {
				t.Errorf("got generated file contents %q, want %q", actual, expected)
			}

			return nil
		}
		defer utils.ResetSystemFunctions()

		fsys := fstest.MapFS{
			idl.Step_stats.String():                                                                {Mode: os.ModeDir},
			filepath.Join(idl.Step_stats.String(), "database_stats"):                               {Mode: os.ModeDir},
			filepath.Join(idl.Step_stats.String(), "database_stats", "generate_database_stats.sh"): {},
		}

		err := commanders.GenerateMigrationScript(idl.Step_stats, seedDir, fsys, outputDir, gphome, port, database)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		if !writeGeneratedScriptCalled {
			t.Error("expected writeFile to be called for generated script")
		}
	})

	t.Run("correctly adds the header file", func(t *testing.T) {
		commanders.SetPsqlFileCommand(exectest.NewCommand(commanders.SuccessScript))
		defer commanders.ResetPsqlFileCommand()

		utils.System.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		defer utils.ResetSystemFunctions()

		writeGeneratedScriptCalled := false
		utils.System.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writeGeneratedScriptCalled = true

			expected := filepath.Join(outputDir, "current", phase.String(), "gphdfs_user_roles", "migration_postgres_gen_alter_gphdfs_roles.sql")
			if filename != expected {
				t.Errorf("got filename %q, want %q", filename, expected)
			}

			expected = "\\c postgres\ngphdfs roles header\nsuccessfully executed data migration SQL script"
			actual := string(data)
			if actual != expected {
				t.Errorf("got generated file contents %q, want %q", actual, expected)
			}

			return nil
		}
		defer utils.ResetSystemFunctions()

		err := commanders.GenerateMigrationScript(phase, seedDir, fsys, outputDir, gphome, port, database)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		if !writeGeneratedScriptCalled {
			t.Error("expected writeFile to be called for generated script")
		}
	})

	t.Run("errors when failing to read header", func(t *testing.T) {
		commanders.SetPsqlFileCommand(exectest.NewCommand(commanders.SuccessScript))
		defer commanders.ResetPsqlFileCommand()

		expected := os.ErrPermission
		utils.System.ReadFileFS = func(f fs.FS, s string) ([]byte, error) {
			return nil, expected
		}
		defer utils.ResetSystemFunctions()

		err := commanders.GenerateMigrationScript(phase, seedDir, fsys, outputDir, gphome, port, database)
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("got error %#v want %#v", err, os.ErrPermission)
		}
	})

	t.Run("errors when failing to make script directory", func(t *testing.T) {
		commanders.SetPsqlFileCommand(exectest.NewCommand(commanders.SuccessScript))
		defer commanders.ResetPsqlFileCommand()

		expected := os.ErrPermission
		utils.System.MkdirAll = func(path string, perm os.FileMode) error {
			return expected
		}
		defer utils.ResetSystemFunctions()

		utils.System.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return nil
		}
		defer utils.ResetSystemFunctions()

		err := commanders.GenerateMigrationScript(phase, seedDir, fsys, outputDir, gphome, port, database)
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("got error %#v want %#v", err, os.ErrPermission)
		}
	})

	t.Run("errors when failing to write generated sql script", func(t *testing.T) {
		commanders.SetPsqlFileCommand(exectest.NewCommand(commanders.SuccessScript))
		defer commanders.ResetPsqlFileCommand()

		utils.System.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		defer utils.ResetSystemFunctions()

		expected := os.ErrPermission
		utils.System.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return expected
		}
		defer utils.ResetSystemFunctions()

		err := commanders.GenerateMigrationScript(phase, seedDir, fsys, outputDir, gphome, port, database)
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("got error %#v want %#v", err, os.ErrPermission)
		}
	})
}

func TestGetDatabases(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("couldn't create sqlmock: %v", err)
	}
	defer testutils.FinishMock(mock, t)

	t.Run("succeeds", func(t *testing.T) {
		expectPgDatabaseToReturn(mock).WillReturnRows(sqlmock.NewRows([]string{"datname", "quoted_datname"}).
			AddRow("template1", "template1").
			AddRow("postgres", "postgres"))

		databases, err := commanders.GetDatabases(db)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		expected := []commanders.DatabaseName{
			{QuotedDatname: "template1", Datname: "template1"},
			{QuotedDatname: "postgres", Datname: "postgres"}}
		if !reflect.DeepEqual(databases, expected) {
			t.Errorf("got %v, want %v", databases, expected)
		}
	})

	t.Run("errors when failing to query", func(t *testing.T) {
		expected := os.ErrPermission
		expectPgDatabaseToReturn(mock).WillReturnError(expected)

		databases, err := commanders.GetDatabases(db)
		if !errors.Is(err, expected) {
			t.Errorf("got %v want %v", err, expected)
		}

		if databases != nil {
			t.Error("expected nil databases")
		}
	})

	t.Run("errors when failing to scan", func(t *testing.T) {
		expectPgDatabaseToReturn(mock).WillReturnRows(sqlmock.NewRows([]string{}).
			AddRow()) // return less fields than scan expects

		databases, err := commanders.GetDatabases(db)
		if !strings.Contains(err.Error(), "Scan") {
			t.Errorf(`expected %v to contain "Scan"`, err)
		}

		if databases != nil {
			t.Error("expected nil databases")
		}
	})

	t.Run("errors when iterating the rows cals", func(t *testing.T) {
		expected := os.ErrPermission
		expectPgDatabaseToReturn(mock).WillReturnRows(sqlmock.NewRows([]string{"datname"}).
			AddRow("postgres").
			RowError(0, expected))

		databases, err := commanders.GetDatabases(db)
		if !errors.Is(err, expected) {
			t.Errorf("got %v want %v", err, expected)
		}

		if databases != nil {
			t.Error("expected nil databases")
		}
	})
}

func expectPgDatabaseToReturn(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(`SELECT datname, quote_ident\(datname\) AS quoted_datname FROM pg_database WHERE datname != 'template0';`)
}
