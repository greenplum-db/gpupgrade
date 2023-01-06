// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commanders

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
)

var MigrationScriptPhases = []idl.Step{idl.Step_initialize, idl.Step_finalize, idl.Step_revert, idl.Step_stats}

var psqlCommand = exec.Command

func SetPsqlCommand(command exectest.Command) {
	psqlCommand = command
}

func ResetPsqlCommand() {
	psqlCommand = exec.Command
}

func executeSQLCommand(gphome string, port int, database string, sql string) ([]byte, error) {
	cmd := psqlCommand(filepath.Join(gphome, "bin", "psql"), "--no-psqlrc", "--quiet",
		"-d", database,
		"-p", strconv.Itoa(port),
		"-c", sql)
	cmd.Env = []string{}

	log.Printf("Executing: %q", cmd.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%q failed with %q: %w", cmd.String(), string(output), err)
	}

	return output, nil
}

var psqlFileCommand = exec.Command

func SetPsqlFileCommand(command exectest.Command) {
	psqlFileCommand = command
}

func ResetPsqlFileCommand() {
	psqlFileCommand = exec.Command
}

func applySQLFile(gphome string, port int, database string, path string, args ...string) ([]byte, error) {
	args = append(args,
		"--no-psqlrc", "--quiet",
		"-d", database,
		"-p", strconv.Itoa(port),
		"-f", path)

	cmd := psqlFileCommand(filepath.Join(gphome, "bin", "psql"), args...)
	cmd.Env = []string{}

	log.Printf("Executing: %q", cmd.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%q failed with %q: %w", cmd.String(), string(output), err)
	}

	return output, nil
}

var bashCommand = exec.Command

func SetBashCommand(command exectest.Command) {
	bashCommand = command
}

func ResetBashCommand() {
	bashCommand = exec.Command
}

func executeBashFile(gphome string, port int, path string, database string) ([]byte, error) {
	cmd := bashCommand(path, gphome, strconv.Itoa(port), database)
	cmd.Env = []string{}

	log.Printf("Executing: %q", cmd.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%q failed with %q: %w", cmd.String(), string(output), err)
	}

	return output, nil
}
