// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"io"
	"log"
	"os/exec"
	"path/filepath"

	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"
)

const DefaultHubPort = 7527
const DefaultAgentPort = 6416
const DefaultDynamicLibraryPath = "$libdir"

var pgupgradeCmd = exec.Command

func Run(stdout, stderr io.Writer, opts *idl.PgOptions) error {
	upgradeDir, err := utils.GetPgUpgradeDir(
		opts.GetRole(),
		opts.GetContentID(),
		opts.GetPgUpgradeTimeStamp(),
	)
	if err != nil {
		return err
	}

	err = utils.System.MkdirAll(upgradeDir, 0700)
	if err != nil {
		return err
	}

	args := []string{
		"--retain",
		"--progress",
		"--old-bindir", opts.GetOldBinDir(),
		"--new-bindir", opts.GetNewBinDir(),
		"--old-datadir", opts.GetOldDataDir(),
		"--new-datadir", opts.GetNewDataDir(),
		"--old-port", opts.GetOldPort(),
		"--new-port", opts.GetNewPort(),
		"--mode", opts.GetPgUpgradeMode().String(),
		"--jobs", opts.GetPgUpgradeJobs(),
		"--output-dir", upgradeDir,
	}

	if opts.GetPgUpgradeVerbose() {
		args = append(args, "--verbose")
	}

	if opts.GetSkipPgUpgradeChecks() {
		args = append(args, "--skip-checks")
	}

	if opts.GetAction() == idl.PgOptions_check {
		args = append(args, "--check")
		args = append(args, "--continue-check-on-fatal")
	}

	if opts.GetMode() == idl.Mode_link {
		args = append(args, "--link")
	}

	if opts.GetOldOptions() != "" {
		args = append(args, "--old-options", opts.GetOldOptions())
	}

	// Below 7X, specify the dbid's for upgrading tablespaces.
	if semver.MustParse(opts.GetTargetVersion()).Major < 7 {
		if opts.GetAction() != idl.PgOptions_check {
			args = append(args, "--old-tablespaces-file", utils.GetOldTablespacesFile(opts.GetBackupDir()))
		}

		args = append(args, "--old-gp-dbid", opts.GetOldDBID())
		args = append(args, "--new-gp-dbid", opts.GetNewDBID())
	}

	utility := filepath.Join(opts.GetNewBinDir(), "pg_upgrade")
	cmd := pgupgradeCmd(utility, args...)

	cmd.Dir = upgradeDir
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	// Explicitly clear the child environment. pg_upgrade shouldn't need things
	// like PATH and PGPORT which are explicitly forbidden to be set.
	cmd.Env = []string{}

	log.Printf("Executing: %q", cmd.String())

	return cmd.Run()
}

func SetPgUpgradeCommand(cmdFunc exectest.Command) {
	pgupgradeCmd = cmdFunc
}

func ResetPgUpgradeCommand() {
	pgupgradeCmd = nil
}
