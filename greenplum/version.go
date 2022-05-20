// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
)

var versionCommand = exec.Command

// XXX: for internal testing only
func SetVersionCommand(command exectest.Command) {
	versionCommand = command
}

// XXX: for internal testing only
func ResetVersionCommand() {
	versionCommand = exec.Command
}

func Version(gphome string) (semver.Version, error) {
	cmd := versionCommand(filepath.Join(gphome, "bin", "postgres"), "--gp-version")
	cmd.Env = []string{}

	gplog.Debug(cmd.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return semver.Version{}, fmt.Errorf("%q failed with %q: %w", cmd.String(), string(output), err)
	}

	rawVersion := string(output)
	parts := strings.SplitN(strings.TrimSpace(rawVersion), "postgres (Greenplum Database) ", 2)
	if len(parts) != 2 {
		return semver.Version{}, xerrors.Errorf(`Greenplum version %q is not of the form "postgres (Greenplum Database) #.#.#"`, rawVersion)
	}

	pattern := regexp.MustCompile(`\d+\.\d+\.\d+`)
	matches := pattern.FindStringSubmatch(parts[1])
	if len(matches) < 1 {
		return semver.Version{}, xerrors.Errorf("parsing Greenplum version %q: %w", rawVersion, err)
	}

	version, err := semver.Parse(matches[0])
	if err != nil {
		return semver.Version{}, xerrors.Errorf("parsing Greenplum version %q: %w", rawVersion, err)
	}

	return version, nil
}
