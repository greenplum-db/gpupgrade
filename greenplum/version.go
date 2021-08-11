// Copyright (c) 2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

var ErrUnknownVersion = errors.New("unknown GPDB version")

type versions struct {
	targetGPHome string
}

func NewVersions(TargetGPHome string) *versions {
	return &versions{targetGPHome: TargetGPHome}
}

func (v *versions) Description() string {
	return "Greenplum Database"
}

func (v *versions) Local() (string, error) {
	version, err := version(v.targetGPHome, "")
	if err != nil {
		return "", err
	}

	return version.String(), err
}

func (v *versions) Remote(host string) (string, error) {
	version, err := version(v.targetGPHome, host)
	if err != nil {
		return "", err
	}

	return version.String(), err
}

func LocalVersion(gphome string) (semver.Version, error) {
	version, err := version(gphome, "")
	if err != nil {
		return semver.Version{}, err
	}

	return version, err
}

func version(gphome string, host string) (semver.Version, error) {
	postgres := filepath.Join(gphome, "bin", "postgres")

	name := postgres
	args := []string{"--gp-version"}
	if host != "" {
		name = "ssh"
		args = []string{"-q", host, fmt.Sprintf(`bash -c "%s --gp-version"`, postgres)}
	}

	cmd := execCommand(name, args...)
	cmd.Env = []string{} // explicitly clear the environment

	gplog.Debug("running cmd %q", cmd.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return semver.Version{}, xerrors.Errorf("%q failed with %q: %w", cmd.String(), string(output), err)
	}

	version := string(output)
	gplog.Debug("version: %q", version)
	return parseVersion(version)
}

// parseVersion takes the output from `postgres --gp-version` and returns the
// parsed dotted-triple semantic version.
func parseVersion(gpversion string) (semver.Version, error) {
	// XXX The following logic is based on dbconn.InitializeVersion, in an
	// attempt to minimize implementation differences between this and the
	// version that is parsed from a live cluster. We can't use that logic
	// as-is, unfortunately, because the version string formats aren't the same
	// for the two cases:
	//
	//     postgres=# select version();
	//
	//                               version
	//     -----------------------------------------------------------
	//      PostgreSQL 8.3.23 (Greenplum Database 5.0.0 build dev) ...
	//     (1 row)
	//
	// versus
	//
	//     $ ${GPHOME}/bin/postgres --gp-version
	//     postgres (Greenplum Database) 5.0.0 build dev
	//
	// Consolidate once the dependency on dbconn is removed from the codebase.
	const marker = "(Greenplum Database)"

	// Remove everything up to and including the marker.
	markerStart := strings.Index(gpversion, marker)
	if markerStart < 0 {
		return semver.Version{}, &unknownVersionError{gpversion}
	}

	versionStart := markerStart + len(marker)
	version := gpversion[versionStart:]

	// Find the dotted triple.
	pattern := regexp.MustCompile(`\d+\.\d+\.\d+`)
	matches := pattern.FindStringSubmatch(version)

	if len(matches) < 1 {
		return semver.Version{}, &unknownVersionError{gpversion}
	}

	return semver.Parse(matches[0])
}

// unknownVersionError is returned when parseVersion fails. It's an instance
// of ErrUnknownVersion.
type unknownVersionError struct {
	input string
}

func (u *unknownVersionError) Error() string {
	return fmt.Sprintf("could not find GPDB version in %q", u.input)
}

func (u *unknownVersionError) Is(err error) bool {
	return err == ErrUnknownVersion
}
