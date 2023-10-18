// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
)

type Version struct {
	Source          string
	Target          string
	Platform        string
	RpmVersion      string
	SpecialJobs     bool
	AppendImageName string
}

type MajorVersions []string

func (a MajorVersions) contains(needle string) bool {
	for _, majorVersion := range a {
		if needle == majorVersion {
			return true
		}
	}

	return false
}

type GPDBVersion struct {
	Version
	GPDBVersion      string
	TestRCIdentifier string
}

type GPDBVersions []GPDBVersion

func (g GPDBVersions) contains(needle GPDBVersion) bool {
	for _, gpdbVersion := range g {
		if (needle.GPDBVersion == gpdbVersion.GPDBVersion) &&
			(needle.Platform == gpdbVersion.Platform) &&
			(needle.RpmVersion == gpdbVersion.RpmVersion) {
			return true
		}
	}

	return false
}

// testRCIdentifier returns the unique identifier used when naming the test
// release candidate RPMs. This is used to prevent bucket filename collisions.
func testRCIdentifier(version string) string {
	fmtString := "%s-%s-"
	identifier := ""

	switch version {
	case "5":
		identifier = fmt.Sprintf(fmtString, os.Getenv("5X_GIT_USER"), os.Getenv("5X_GIT_BRANCH"))
	case "6":
		identifier = fmt.Sprintf(fmtString, os.Getenv("6X_GIT_USER"), os.Getenv("6X_GIT_BRANCH"))
	case "7":
		identifier = fmt.Sprintf(fmtString, os.Getenv("7X_GIT_USER"), os.Getenv("7X_GIT_BRANCH"))
	default:
		return ""
	}

	if identifier == fmt.Sprintf(fmtString, "", "") {
		// If env variables are empty, return empty string rather than the empty fmtString of "--"
		return ""
	}

	return identifier
}
