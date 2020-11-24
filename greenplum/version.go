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
)

var ErrUnknownVersion = errors.New("unknown GPDB version")

type GPDBVersions struct {
	TargetGPHome string
}

func (g *GPDBVersions) HubVersion() (string, error) {
	version, err := GPDBVersion(g.TargetGPHome)
	if err != nil {
		return "", err
	}

	return version.String(), err
}

func (g *GPDBVersions) AgentVersion(host string) (string, error) {
	version, err := GPDBVersionOnHost(g.TargetGPHome, host)
	if err != nil {
		return "", err
	}

	return version.String(), err
}

func GPDBVersion(gphome string) (semver.Version, error) {
	return getGPDBVersion(gphome, "")
}

func GPDBVersionOnHost(gphome string, host string) (semver.Version, error) {
	return getGPDBVersion(gphome, host)
}

// GPDBVersion returns the semantic version of a GPDB installation located at
// the given GPHOME.
func getGPDBVersion(gphome string, host string) (semver.Version, error) {
	postgres := filepath.Join(gphome, "bin", "postgres")

	name := postgres
	args := []string{"--gp-version"}
	if host != "" {
		name = "ssh"
		args = []string{host, fmt.Sprintf(`bash -c "%s --gp-version"`, postgres)}
	}

	cmd := execCommand(name, args...)
	cmd.Env = []string{} // explicitly clear the environment

	stdout, err := cmd.Output()
	if err != nil {
		return semver.Version{}, err
	}

	version := string(stdout)
	return parseGPVersion(version)
}

// parseGPVersion takes the output from `postgres --gp-version` and returns the
// parsed dotted-triple semantic version.
func parseGPVersion(gpversion string) (semver.Version, error) {
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

// unknownVersionError is returned when parseGPVersion fails. It's an instance
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
