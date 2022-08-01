// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
)

type Version struct {
	sourceVersion string
	targetVersion string
	centosVersion string
	SpecialJobs   bool
}

// testRCIdentifier returns the unique identifier used when naming the test
// release candidate RPMs. This is used to to prevent bucket filename collisions.
func (v *Version) testRCIdentifier() string {
	fmtString := "%s-%s-"
	identifier := ""
	switch v.sourceVersion {
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
	CentosVersion    string
	GPDBVersion      string
	TestRCIdentifier string
}

type GPDBVersions []GPDBVersion

func (v GPDBVersions) contains(needle GPDBVersion) bool {
	for _, version := range v {
		if needle == version {
			return true
		}
	}

	return false
}
