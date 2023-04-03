// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commanders_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestApplyDataMigrationScripts(t *testing.T) {
	logDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, logDir)

	currentScriptDir := "/home/gpupgrade/data-migration/current"

	currentDirFS := fstest.MapFS{
		idl.Step_stats.String():                                  {Mode: os.ModeDir},
		filepath.Join(idl.Step_stats.String(), "generate_stats"): {Mode: os.ModeDir},
		filepath.Join(idl.Step_stats.String(), "generate_stats", "migration_postgres_generate_stats.sql"):  {},
		filepath.Join(idl.Step_stats.String(), "generate_stats", "migration_template1_generate_stats.sql"): {},
	}

	t.Run("returns when there are no scripts to apply", func(t *testing.T) {
		err := commanders.ApplyDataMigrationScripts(false, "", 0, logDir, currentDirFS, "", idl.Step_revert)
		if err != nil {
			t.Fatalf("unexpected error %#v", err)
		}
	})

	t.Run("prints stats specific message for stats phase", func(t *testing.T) {
		d := commanders.BufferStandardDescriptors(t)

		currentScriptDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, currentScriptDir)

		utils.System.DirFS = func(dir string) fs.FS {
			return currentDirFS
		}
		defer utils.ResetSystemFunctions()

		resetStdin := testutils.SetStdin(t, "a\n")
		defer resetStdin()

		err := commanders.ApplyDataMigrationScripts(false, "", 0, logDir, currentDirFS, currentScriptDir, idl.Step_stats)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		stdout, stderr := d.Collect()
		d.Close()
		if len(stderr) != 0 {
			t.Errorf("unexpected stderr %#v", string(stderr))
		}

		expected := "To receive an upgrade time estimate send the stats output"
		actual := string(stdout)
		if !strings.Contains(actual, expected) {
			t.Errorf("expected output %#v to contain %#v", actual, expected)
			t.Logf("actual:   %#v", actual)
			t.Logf("expected: %#v", expected)
		}
	})

	t.Run("does not error when prompt returns skipped", func(t *testing.T) {
		resetStdin := testutils.SetStdin(t, "n\n")
		defer resetStdin()

		err := commanders.ApplyDataMigrationScripts(false, "", 0, logDir, currentDirFS, currentScriptDir, idl.Step_stats)
		if err != nil {
			t.Fatalf("unexpected error %#v", err)
		}
	})

	t.Run("errors when prompt fails", func(t *testing.T) {
		expected := os.ErrPermission
		utils.System.ReadDirFS = func(fsys fs.FS, name string) ([]fs.DirEntry, error) {
			return nil, expected
		}
		defer utils.ResetSystemFunctions()

		err := commanders.ApplyDataMigrationScripts(false, "", 0, logDir, currentDirFS, currentScriptDir, idl.Step_stats)
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v want %#v", err, expected)
		}
	})

	t.Run("errors when applying script sub directory fails", func(t *testing.T) {
		commanders.SetPsqlFileCommand(exectest.NewCommand(commanders.FailedMain))
		defer commanders.ResetPsqlFileCommand()

		currentScriptDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, currentScriptDir)

		// This is mocking scriptDirFS
		utils.System.DirFS = func(dir string) fs.FS {
			return fstest.MapFS{
				idl.Step_stats.String():                                  {Mode: os.ModeDir},
				filepath.Join(idl.Step_stats.String(), "generate_stats"): {Mode: os.ModeDir},
				"migration_postgres_generate_stats.sql":                  {},
			}
		}
		defer utils.ResetSystemFunctions()

		resetStdin := testutils.SetStdin(t, "a\n")
		defer resetStdin()

		err := commanders.ApplyDataMigrationScripts(false, "", 0, logDir, currentDirFS, currentScriptDir, idl.Step_stats)
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Errorf("got %T, want %T", err, exitError)
		}
	})

	t.Run("errors when failing to open output log", func(t *testing.T) {
		utils.System.DirFS = func(dir string) fs.FS {
			return fstest.MapFS{
				idl.Step_stats.String(): {Mode: os.ModeDir},
			}
		}
		defer utils.ResetSystemFunctions()

		expected := os.ErrPermission
		utils.System.OpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
			return nil, expected
		}
		defer utils.ResetSystemFunctions()

		resetStdin := testutils.SetStdin(t, "a\n")
		defer resetStdin()

		err := commanders.ApplyDataMigrationScripts(false, "", 0, logDir, currentDirFS, currentScriptDir, idl.Step_stats)
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("got error %#v want %#v", err, os.ErrPermission)
		}
	})
}

func TestApplyDataMigrationScriptSubDir(t *testing.T) {
	scriptSubDir := "/home/gpupgrade/data-migration/current/initialize/unique_primary_foreign_key_constraint"

	t.Run("errors when failing to read current script directory", func(t *testing.T) {
		utils.System.ReadDirFS = func(fsys fs.FS, name string) ([]fs.DirEntry, error) {
			return nil, os.ErrPermission
		}
		defer utils.ResetSystemFunctions()

		output, err := commanders.ApplyDataMigrationScriptSubDir("", 0, fstest.MapFS{}, scriptSubDir)
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("got error %#v want %#v", err, os.ErrPermission)
		}

		if output != nil {
			t.Error("expected nil output")
		}
	})

	t.Run("errors when no directories are in the current script directory", func(t *testing.T) {
		output, err := commanders.ApplyDataMigrationScriptSubDir("", 0, fstest.MapFS{}, scriptSubDir)
		expected := fmt.Sprintf("No SQL files found in %q.", scriptSubDir)
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}

		if output != nil {
			t.Error("expected nil output")
		}
	})

	t.Run("only applies sql files", func(t *testing.T) {
		commanders.SetPsqlFileCommand(exectest.NewCommand(commanders.SuccessScript))
		defer commanders.ResetPsqlFileCommand()

		fsys := fstest.MapFS{
			"some_directory": {Mode: os.ModeDir},
			"migration_postgres_gen_drop_constraint_2_primary_unique.sql": {},
			"drop_postgres_indexes.bash":                                  {},
		}

		output, err := commanders.ApplyDataMigrationScriptSubDir("", 0, fsys, scriptSubDir)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		if string(output) != commanders.SuccessScriptOutput {
			t.Errorf("got output %q, want %q", output, commanders.SuccessScriptOutput)
		}
	})

	t.Run("errors when applying sql file fails", func(t *testing.T) {
		commanders.SetPsqlFileCommand(exectest.NewCommand(commanders.FailedMain))
		defer commanders.ResetPsqlFileCommand()

		fsys := fstest.MapFS{
			"migration_postgres_gen_drop_constraint_2_primary_unique.sql": {},
		}

		output, err := commanders.ApplyDataMigrationScriptSubDir("", 0, fsys, scriptSubDir)
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Errorf("got %T, want %T", err, exitError)
		}

		if output != nil {
			t.Error("expected nil output")
		}
	})
}

func TestApplyDataMigrationScriptsPrompt(t *testing.T) {
	currentScriptDir := "/home/gpupgrade/data-migration/current"
	phase := idl.Step_initialize

	fsys := fstest.MapFS{
		idl.Step_initialize.String(): {Mode: os.ModeDir},
		filepath.Join(idl.Step_initialize.String(), "unique_primary_foreign_key_constraint"):                                                                    {Mode: os.ModeDir},
		filepath.Join(idl.Step_initialize.String(), "unique_primary_foreign_key_constraint", "migration_postgres_gen_drop_constraint_2_primary_unique.sql"):     {},
		filepath.Join(idl.Step_initialize.String(), "unique_primary_foreign_key_constraint", "migration_testDB_gen_drop_constraint_2_primary_unique.sql"):       {},
		filepath.Join(idl.Step_initialize.String(), "parent_partitions_with_seg_entries"):                                                                       {Mode: os.ModeDir},
		filepath.Join(idl.Step_initialize.String(), "parent_partitions_with_seg_entries", "migration_postgres_gen_drop_parent_partitions_with_seg_entries.sql"): {},
		filepath.Join(idl.Step_initialize.String(), "parent_partitions_with_seg_entries", "migration_testDB_gen_drop_parent_partitions_with_seg_entries.sql"):   {},
	}

	t.Run("errors when failing to read input", func(t *testing.T) {
		reader := bufio.NewReader(strings.NewReader(""))
		actualScriptDirs, err := commanders.ApplyDataMigrationScriptsPrompt(false, reader, currentScriptDir, fsys, phase)
		expected := io.EOF
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}

		if actualScriptDirs != nil {
			t.Error("expected nil script directories")
		}
	})

	t.Run("applies all scripts when user selects 'a'll", func(t *testing.T) {
		reader := bufio.NewReader(strings.NewReader("a\n"))
		actualScriptDirs, err := commanders.ApplyDataMigrationScriptsPrompt(false, reader, currentScriptDir, fsys, phase)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		expectedScriptDirs := []string{"/home/gpupgrade/data-migration/current/initialize/parent_partitions_with_seg_entries", "/home/gpupgrade/data-migration/current/initialize/unique_primary_foreign_key_constraint"}
		if !reflect.DeepEqual(actualScriptDirs, expectedScriptDirs) {
			t.Errorf("got %s, want %s", actualScriptDirs, expectedScriptDirs)
		}
	})

	t.Run("errors when applies all scripts fails to read phase directory in current generated script directory", func(t *testing.T) {
		reader := bufio.NewReader(strings.NewReader("a\n"))
		actualScriptDirs, err := commanders.ApplyDataMigrationScriptsPrompt(false, reader, currentScriptDir, fsys, idl.Step_unknown_step)
		var expected *os.PathError
		if !errors.As(err, &expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}

		if actualScriptDirs != nil {
			t.Error("expected nil script directories")
		}
	})

	t.Run("does not prompt and applies all scripts when in non-interactive mode", func(t *testing.T) {
		d := commanders.BufferStandardDescriptors(t)

		actualScriptDirs, err := commanders.ApplyDataMigrationScriptsPrompt(true, nil, currentScriptDir, fsys, phase)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		expectedScriptDirs := []string{"/home/gpupgrade/data-migration/current/initialize/parent_partitions_with_seg_entries", "/home/gpupgrade/data-migration/current/initialize/unique_primary_foreign_key_constraint"}
		if !reflect.DeepEqual(actualScriptDirs, expectedScriptDirs) {
			t.Errorf("got %s, want %s", actualScriptDirs, expectedScriptDirs)
		}

		stdout, stderr := d.Collect()
		d.Close()
		if len(stderr) != 0 {
			t.Errorf("unexpected stderr %#v", string(stderr))
		}

		expected := "\nApplying 'all' of the \"initialize\" data migration scripts.\n"
		actual := string(stdout)
		if !strings.Contains(actual, expected) {
			t.Errorf("expected output %#v to contain %#v", actual, expected)
			t.Logf("actual:   %#v", actual)
			t.Logf("expected: %#v", expected)
		}
	})

	t.Run("returns error when selecting some scripts when user selects 's'ome with bad input", func(t *testing.T) {
		d := commanders.BufferStandardDescriptors(t)

		reader := bufio.NewReader(strings.NewReader("s\nb\n"))
		actualScriptDirs, err := commanders.ApplyDataMigrationScriptsPrompt(false, reader, currentScriptDir, fsys, phase)
		if !errors.Is(err, io.EOF) {
			t.Errorf("got error %#v, want %#v", err, io.EOF)
		}

		if actualScriptDirs != nil {
			t.Error("expected nil script directories")
		}

		stdout, stderr := d.Collect()
		d.Close()
		if len(stderr) != 0 {
			t.Errorf("unexpected stderr %#v", string(stderr))
		}

		expected := "\nScripts to apply:\n"
		expected += "  parent_partitions_with_seg_entries\n"
		expected += "  - Fixes non-empty segment relfiles for AO and AOCO parent partitions\n\n"
		expected += "  unique_primary_foreign_key_constraint\n"
		expected += "  - Drops constraints\n\n"
		expected += "Which \"initialize\" data migration SQL scripts to apply?\n\n"
		expected += "WARNING: Data migration scripts can leave the source cluster in a non-optimal state \n"
		expected += "         and can take time to fully revert.\n\n"
		expected += "  [n]o scripts.   When running 'before' the upgrade to uncover pg_upgrade --check errors \n"
		expected += "                  there is no need to run the data migration SQL scripts.\n"
		expected += "  [s]ome scripts. Usually run 'before' the upgrade during maintenance windows to run \n"
		expected += "                  selected scripts as suggested in the documentation.\n"
		expected += "  [a]ll scripts.  Usually run 'during' the upgrade within the downtime window.\n"
		expected += "  [q]uit\n"
		expected += "\nSelect: \n"
		expected += "Select scripts to apply separated by commas such as 1, 3. Or [q]uit?\n\n"
		expected += "  0: parent_partitions_with_seg_entries\n"
		expected += "  1: unique_primary_foreign_key_constraint\n\n"
		expected += "Select: "

		actual := string(stdout)
		if actual != expected {
			t.Errorf("expected output %#v to contain %#v", actual, expected)
			t.Logf("actual:   %#v", actual)
			t.Logf("expected: %#v", expected)
		}
	})

	t.Run("returns skip error when user selects 'n'one", func(t *testing.T) {
		reader := bufio.NewReader(strings.NewReader("n\n"))
		actualScriptDirs, err := commanders.ApplyDataMigrationScriptsPrompt(false, reader, currentScriptDir, fsys, phase)
		expected := step.Skip
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}

		if actualScriptDirs != nil {
			t.Error("expected nil scripts")
		}
	})

	t.Run("returns canceled error when user selects 'q'uit", func(t *testing.T) {
		reader := bufio.NewReader(strings.NewReader("q\n"))
		actualScriptDirs, err := commanders.ApplyDataMigrationScriptsPrompt(false, reader, currentScriptDir, fsys, phase)
		expected := step.UserCanceled
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}

		if actualScriptDirs != nil {
			t.Error("expected nil scripts")
		}
	})

	t.Run("re-prompts when user enters 'b'ad input", func(t *testing.T) {
		d := commanders.BufferStandardDescriptors(t)

		reader := bufio.NewReader(strings.NewReader("b\nq\n"))
		actualScriptDirs, err := commanders.ApplyDataMigrationScriptsPrompt(false, reader, currentScriptDir, fsys, phase)
		if !errors.Is(err, step.UserCanceled) {
			t.Errorf("got error %#v, want %#v", err, step.UserCanceled)
		}

		if actualScriptDirs != nil {
			t.Error("expected nil script directories")
		}

		stdout, stderr := d.Collect()
		d.Close()
		if len(stderr) != 0 {
			t.Errorf("unexpected stderr %#v", string(stderr))
		}

		expected := "\nScripts to apply:\n"
		expected += "  parent_partitions_with_seg_entries\n"
		expected += "  - Fixes non-empty segment relfiles for AO and AOCO parent partitions\n\n"
		expected += "  unique_primary_foreign_key_constraint\n"
		expected += "  - Drops constraints\n\n"
		expected += "Which \"initialize\" data migration SQL scripts to apply?\n\n"
		expected += "WARNING: Data migration scripts can leave the source cluster in a non-optimal state \n"
		expected += "         and can take time to fully revert.\n\n"
		expected += "  [n]o scripts.   When running 'before' the upgrade to uncover pg_upgrade --check errors \n"
		expected += "                  there is no need to run the data migration SQL scripts.\n"
		expected += "  [s]ome scripts. Usually run 'before' the upgrade during maintenance windows to run \n"
		expected += "                  selected scripts as suggested in the documentation.\n"
		expected += "  [a]ll scripts.  Usually run 'during' the upgrade within the downtime window.\n"
		expected += "  [q]uit\n"
		expected += "\nSelect: "
		expected += "Which \"initialize\" data migration SQL scripts to apply?\n\n"
		expected += "WARNING: Data migration scripts can leave the source cluster in a non-optimal state \n"
		expected += "         and can take time to fully revert.\n\n"
		expected += "  [n]o scripts.   When running 'before' the upgrade to uncover pg_upgrade --check errors \n"
		expected += "                  there is no need to run the data migration SQL scripts.\n"
		expected += "  [s]ome scripts. Usually run 'before' the upgrade during maintenance windows to run \n"
		expected += "                  selected scripts as suggested in the documentation.\n"
		expected += "  [a]ll scripts.  Usually run 'during' the upgrade within the downtime window.\n"
		expected += "  [q]uit\n"
		expected += "\nSelect: \n"
		expected += "Quitting...\n"

		actual := string(stdout)
		if actual != expected {
			t.Errorf("expected output %#v to contain %#v", actual, expected)
			t.Logf("actual:   %#v", actual)
			t.Logf("expected: %#v", expected)
		}
	})

	t.Run("when phase is initialize it displays warning and additional text on when to run scripts", func(t *testing.T) {
		d := commanders.BufferStandardDescriptors(t)

		reader := bufio.NewReader(strings.NewReader("q\n"))
		actualScriptDirs, err := commanders.ApplyDataMigrationScriptsPrompt(false, reader, currentScriptDir, fsys, phase)
		if !errors.Is(err, step.UserCanceled) {
			t.Errorf("got error %#v, want %#v", err, step.UserCanceled)
		}

		if actualScriptDirs != nil {
			t.Error("expected nil script directories")
		}

		stdout, stderr := d.Collect()
		d.Close()
		if len(stderr) != 0 {
			t.Errorf("unexpected stderr %#v", string(stderr))
		}

		expected := "\nScripts to apply:\n"
		expected += "  parent_partitions_with_seg_entries\n"
		expected += "  - Fixes non-empty segment relfiles for AO and AOCO parent partitions\n\n"
		expected += "  unique_primary_foreign_key_constraint\n"
		expected += "  - Drops constraints\n\n"
		expected += "Which \"initialize\" data migration SQL scripts to apply?\n\n"
		expected += "WARNING: Data migration scripts can leave the source cluster in a non-optimal state \n"
		expected += "         and can take time to fully revert.\n\n"
		expected += "  [n]o scripts.   When running 'before' the upgrade to uncover pg_upgrade --check errors \n"
		expected += "                  there is no need to run the data migration SQL scripts.\n"
		expected += "  [s]ome scripts. Usually run 'before' the upgrade during maintenance windows to run \n"
		expected += "                  selected scripts as suggested in the documentation.\n"
		expected += "  [a]ll scripts.  Usually run 'during' the upgrade within the downtime window.\n"
		expected += "  [q]uit\n"
		expected += "\nSelect: \n"
		expected += "Quitting...\n"

		actual := string(stdout)
		if actual != expected {
			t.Errorf("expected output %#v to contain %#v", actual, expected)
			t.Logf("actual:   %#v", actual)
			t.Logf("expected: %#v", expected)
		}
	})

	t.Run("when phase is 'not' initialize it does 'not' display a warning or additional text", func(t *testing.T) {
		currentScriptDir := "/home/gpupgrade/data-migration/current"
		phase := idl.Step_finalize

		fsys := fstest.MapFS{
			phase.String(): {Mode: os.ModeDir},
			filepath.Join(phase.String(), "partitioned_tables_indexes"):                                                             {Mode: os.ModeDir},
			filepath.Join(phase.String(), "partitioned_tables_indexes", "migration_postgres_recreate_partition_indexes_step_1.sql"): {},
			filepath.Join(phase.String(), "partitioned_tables_indexes", "migration_postgres_recreate_partition_indexes_step_2.sql"): {},
		}

		d := commanders.BufferStandardDescriptors(t)

		reader := bufio.NewReader(strings.NewReader("q\n"))
		actualScriptDirs, err := commanders.ApplyDataMigrationScriptsPrompt(false, reader, currentScriptDir, fsys, phase)
		if !errors.Is(err, step.UserCanceled) {
			t.Errorf("got error %#v, want %#v", err, step.UserCanceled)
		}

		if actualScriptDirs != nil {
			t.Error("expected nil script directories")
		}

		stdout, stderr := d.Collect()
		d.Close()
		if len(stderr) != 0 {
			t.Errorf("unexpected stderr %#v", string(stderr))
		}

		expected := "\nScripts to apply:\n"
		expected += "  partitioned_tables_indexes\n"
		expected += "  - Drops partition indexes\n\n"
		expected += "Which \"finalize\" data migration SQL scripts to apply?\n"
		expected += "  [n]one\n"
		expected += "  [s]ome\n"
		expected += "  [a]ll\n"
		expected += "  [q]uit\n"
		expected += "\nSelect: \n"
		expected += "Quitting...\n"

		actual := string(stdout)
		if actual != expected {
			t.Errorf("expected output %#v to contain %#v", actual, expected)
			t.Logf("actual:   %#v", actual)
			t.Logf("expected: %#v", expected)
		}
	})
}

func TestSelectDataMigrationScriptsPrompt(t *testing.T) {
	currentScriptDir := "/home/gpupgrade/data-migration/current"
	phase := idl.Step_initialize

	fsys := fstest.MapFS{
		idl.Step_initialize.String(): {Mode: os.ModeDir},
		filepath.Join(idl.Step_initialize.String(), "unique_primary_foreign_key_constraint"):                                                                    {Mode: os.ModeDir},
		filepath.Join(idl.Step_initialize.String(), "unique_primary_foreign_key_constraint", "migration_postgres_gen_drop_constraint_2_primary_unique.sql"):     {},
		filepath.Join(idl.Step_initialize.String(), "unique_primary_foreign_key_constraint", "migration_testDB_gen_drop_constraint_2_primary_unique.sql"):       {},
		filepath.Join(idl.Step_initialize.String(), "parent_partitions_with_seg_entries"):                                                                       {Mode: os.ModeDir},
		filepath.Join(idl.Step_initialize.String(), "parent_partitions_with_seg_entries", "migration_postgres_gen_drop_parent_partitions_with_seg_entries.sql"): {},
		filepath.Join(idl.Step_initialize.String(), "parent_partitions_with_seg_entries", "migration_testDB_gen_drop_parent_partitions_with_seg_entries.sql"):   {},
	}

	t.Run("errors when failing to read current script directory", func(t *testing.T) {
		reader := bufio.NewReader(strings.NewReader(""))
		scriptDirs, err := commanders.SelectDataMigrationScriptsPrompt(reader, currentScriptDir, fstest.MapFS{}, phase)
		var expected *os.PathError
		if !errors.As(err, &expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}

		if scriptDirs != nil {
			t.Error("expected nil scripts")
		}
	})

	t.Run("errors when failing to read input", func(t *testing.T) {
		reader := bufio.NewReader(strings.NewReader(""))
		scriptDirs, err := commanders.SelectDataMigrationScriptsPrompt(reader, currentScriptDir, fsys, phase)
		expected := io.EOF
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}

		if scriptDirs != nil {
			t.Error("expected nil scripts")
		}
	})

	t.Run("returns canceled when user selects quit", func(t *testing.T) {
		reader := bufio.NewReader(strings.NewReader("q\n"))
		scriptDirs, err := commanders.SelectDataMigrationScriptsPrompt(reader, currentScriptDir, fsys, phase)
		expected := step.UserCanceled
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}

		if scriptDirs != nil {
			t.Error("expected nil scripts")
		}
	})

	t.Run("prints error and continues when parsing selection fails", func(t *testing.T) {
		d := commanders.BufferStandardDescriptors(t)

		fsys := fstest.MapFS{idl.Step_initialize.String(): {Mode: os.ModeDir}}
		reader := bufio.NewReader(strings.NewReader("0.5\nq\n"))
		scriptDirs, err := commanders.SelectDataMigrationScriptsPrompt(reader, currentScriptDir, fsys, phase)
		if !errors.Is(err, step.UserCanceled) {
			t.Errorf("got error %#v, want %#v", err, step.UserCanceled)
		}

		if scriptDirs != nil {
			t.Error("expected nil scripts")
		}

		stdout, stderr := d.Collect()
		d.Close()
		if len(stderr) != 0 {
			t.Errorf("unexpected stderr %#v", string(stderr))
		}

		expected := "\nSelect scripts to apply separated by commas such as 1, 3. Or [q]uit?\n\n\n"
		expected += "Select: \n"
		expected += "Invalid selection. Found \"0.5\" expected a number or numbers separated by commas such as 1, 3.\n\n"
		expected += "Select scripts to apply separated by commas such as 1, 3. Or [q]uit?\n\n\n"
		expected += "Select: \n"
		expected += "Quitting..."

		actual := string(stdout)
		if actual != expected {
			t.Errorf("expected output %#v to contain %#v", actual, expected)
			t.Logf("actual:   %#v", actual)
			t.Logf("expected: %#v", expected)
		}
	})

	t.Run("errors when failing to read 'continue, edit, or quit' input", func(t *testing.T) {
		reader := bufio.NewReader(strings.NewReader("1\n"))
		scriptDirs, err := commanders.SelectDataMigrationScriptsPrompt(reader, currentScriptDir, fsys, phase)
		expected := io.EOF
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}

		if scriptDirs != nil {
			t.Error("expected nil scripts")
		}
	})

	t.Run("returns selected scripts when user continues", func(t *testing.T) {
		d := commanders.BufferStandardDescriptors(t)

		reader := bufio.NewReader(strings.NewReader("0\ne\n1\nc\n"))
		currentScriptDir := "/home/gpupgrade/data-migration/current"
		actualScriptDirs, err := commanders.SelectDataMigrationScriptsPrompt(reader, currentScriptDir, fsys, phase)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		expectedScriptDirs := []string{"/home/gpupgrade/data-migration/current/initialize/unique_primary_foreign_key_constraint"}
		if !reflect.DeepEqual(actualScriptDirs, expectedScriptDirs) {
			t.Errorf("got %s, want %s", actualScriptDirs, expectedScriptDirs)
		}

		stdout, stderr := d.Collect()
		d.Close()
		if len(stderr) != 0 {
			t.Errorf("unexpected stderr %#v", string(stderr))
		}

		expected := "\nSelect scripts to apply separated by commas such as 1, 3. Or [q]uit?\n\n"
		expected += "  0: parent_partitions_with_seg_entries\n"
		expected += "  1: unique_primary_foreign_key_constraint\n\n"
		expected += "Select: \n"
		expected += "Selected:\n\n"
		expected += "  0: parent_partitions_with_seg_entries\n\n"
		expected += "[c]ontinue, [e]dit selection, or [q]uit.\n"
		expected += "Select: \n"
		expected += "Select scripts to apply separated by commas such as 1, 3. Or [q]uit?\n\n"
		expected += "  0: parent_partitions_with_seg_entries\n"
		expected += "  1: unique_primary_foreign_key_constraint\n\n"
		expected += "Select: \nSelected:\n\n"
		expected += "  1: unique_primary_foreign_key_constraint\n\n"
		expected += "[c]ontinue, [e]dit selection, or [q]uit.\n"
		expected += "Select: \nApplying the \"initialize\" data migration scripts:\n\n"
		expected += "  1: unique_primary_foreign_key_constraint\n\n"

		actual := string(stdout)
		if actual != expected {
			t.Errorf("expected output %#v to contain %#v", actual, expected)
			t.Logf("actual:   %#v", actual)
			t.Logf("expected: %#v", expected)
		}
	})

	t.Run("returns to parse selection when user 'edits' selection", func(t *testing.T) {
		d := commanders.BufferStandardDescriptors(t)

		reader := bufio.NewReader(strings.NewReader("0\ne\n1\nq\n"))
		scriptDirs, err := commanders.SelectDataMigrationScriptsPrompt(reader, currentScriptDir, fsys, phase)
		if !errors.Is(err, step.UserCanceled) {
			t.Errorf("got error %#v, want %#v", err, step.UserCanceled)
		}

		if scriptDirs != nil {
			t.Error("expected nil scripts")
		}

		stdout, stderr := d.Collect()
		d.Close()
		if len(stderr) != 0 {
			t.Errorf("unexpected stderr %#v", string(stderr))
		}

		expected := "\nSelect scripts to apply separated by commas such as 1, 3. Or [q]uit?\n\n"
		expected += "  0: parent_partitions_with_seg_entries\n"
		expected += "  1: unique_primary_foreign_key_constraint\n\n"
		expected += "Select: \n"
		expected += "Selected:\n\n"
		expected += "  0: parent_partitions_with_seg_entries\n\n"
		expected += "[c]ontinue, [e]dit selection, or [q]uit.\n"
		expected += "Select: \n"
		expected += "Select scripts to apply separated by commas such as 1, 3. Or [q]uit?\n\n"
		expected += "  0: parent_partitions_with_seg_entries\n"
		expected += "  1: unique_primary_foreign_key_constraint\n\n"
		expected += "Select: \nSelected:\n\n"
		expected += "  1: unique_primary_foreign_key_constraint\n\n"
		expected += "[c]ontinue, [e]dit selection, or [q]uit.\n"
		expected += "Select: \n"
		expected += "Quitting..."

		actual := string(stdout)
		if actual != expected {
			t.Errorf("expected output %#v to contain %#v", actual, expected)
			t.Logf("actual:   %#v", actual)
			t.Logf("expected: %#v", expected)
		}
	})

	t.Run("returns to parse selection when user makes a 'bad' selection", func(t *testing.T) {
		d := commanders.BufferStandardDescriptors(t)

		reader := bufio.NewReader(strings.NewReader("0\nbad\nq\n"))
		scriptDirs, err := commanders.SelectDataMigrationScriptsPrompt(reader, currentScriptDir, fsys, phase)
		if !errors.Is(err, step.UserCanceled) {
			t.Errorf("got error %#v, want %#v", err, step.UserCanceled)
		}

		if scriptDirs != nil {
			t.Error("expected nil scripts")
		}

		stdout, stderr := d.Collect()
		d.Close()
		if len(stderr) != 0 {
			t.Errorf("unexpected stderr %#v", string(stderr))
		}

		expected := "\nSelect scripts to apply separated by commas such as 1, 3. Or [q]uit?\n\n"
		expected += "  0: parent_partitions_with_seg_entries\n"
		expected += "  1: unique_primary_foreign_key_constraint\n\n"
		expected += "Select: \n"
		expected += "Selected:\n\n"
		expected += "  0: parent_partitions_with_seg_entries\n\n"
		expected += "[c]ontinue, [e]dit selection, or [q]uit.\n"
		expected += "Select: \n"
		expected += "Select scripts to apply separated by commas such as 1, 3. Or [q]uit?\n\n"
		expected += "  0: parent_partitions_with_seg_entries\n"
		expected += "  1: unique_primary_foreign_key_constraint\n\n"
		expected += "Select: \n"
		expected += "Quitting..."

		actual := string(stdout)
		if actual != expected {
			t.Errorf("expected output %#v to contain %#v", actual, expected)
			t.Logf("actual:   %#v", actual)
			t.Logf("expected: %#v", expected)
		}
	})

	t.Run("returns canceled when user selects quit from 'continue, edit, or quit' prompt", func(t *testing.T) {
		reader := bufio.NewReader(strings.NewReader("1\nq\n"))
		scriptDirs, err := commanders.SelectDataMigrationScriptsPrompt(reader, currentScriptDir, fsys, phase)
		expected := step.UserCanceled
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}

		if scriptDirs != nil {
			t.Error("expected nil scripts")
		}
	})
}

func TestParseSelection(t *testing.T) {
	// error cases
	errCases := []struct {
		name     string
		input    string
		expected error
	}{
		{
			name:     "errors when input is empty",
			input:    "",
			expected: fmt.Errorf("Expected a number or numbers separated by commas such as 1, 3."),
		},
		{
			name:     "errors when input is whitespace",
			input:    "  \t   \n",
			expected: fmt.Errorf("Expected a number or numbers separated by commas such as 1, 3."),
		},
		{
			name:     "cancels when input is 'q'",
			input:    "q\n",
			expected: step.UserCanceled,
		},
		{
			name:     "errors when selection is not a number",
			input:    "A",
			expected: fmt.Errorf("Invalid selection. Found %q expected a number or numbers separated by commas such as 1, 3.", "a"),
		},
		{
			name:     "errors when selection is not a positive number",
			input:    "-1",
			expected: fmt.Errorf("Invalid selection. Found %q expected a number or numbers separated by commas such as 1, 3.", "-1"),
		},
		{
			name:     "errors when selection is not a whole number",
			input:    "0.5",
			expected: fmt.Errorf("Invalid selection. Found %q expected a number or numbers separated by commas such as 1, 3.", "0.5"),
		},
	}

	for _, c := range errCases {
		t.Run(c.name, func(t *testing.T) {
			scripts, err := commanders.ParseSelection(c.input, commanders.Scripts{})
			if !reflect.DeepEqual(err, c.expected) {
				t.Errorf("got error %#v, want %#v", err, c.expected)
			}

			if scripts != nil {
				t.Error("expected nil scripts")
			}
		})
	}

	// positive cases
	cases := []struct {
		name     string
		input    string
		expected commanders.Scripts
	}{
		{
			name:     "succeeds when input is a single selection",
			input:    "0",
			expected: commanders.Scripts{commanders.Script{Num: 0, Name: "zero"}},
		},
		{
			name:     "succeeds when input is multiple selections",
			input:    "0,2,3",
			expected: commanders.Scripts{commanders.Script{Num: 0, Name: "zero"}, commanders.Script{Num: 2, Name: "two"}, commanders.Script{Num: 3, Name: "three"}},
		},
		{
			name:     "succeeds when input is multiple selections with whitespace",
			input:    "    0, \t 2 , 3   ",
			expected: commanders.Scripts{commanders.Script{Num: 0, Name: "zero"}, commanders.Script{Num: 2, Name: "two"}, commanders.Script{Num: 3, Name: "three"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			allScripts := commanders.Scripts{
				commanders.Script{Num: 0, Name: "zero"},
				commanders.Script{Num: 1, Name: "one"},
				commanders.Script{Num: 2, Name: "two"},
				commanders.Script{Num: 3, Name: "three"},
			}

			scripts, err := commanders.ParseSelection(c.input, allScripts)
			if err != nil {
				t.Errorf("unexpected err %#v", err)
			}

			if !reflect.DeepEqual(scripts, c.expected) {
				t.Errorf("got %v, want %v", scripts, c.expected)
			}
		})
	}
}
