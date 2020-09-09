// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import "fmt"

// These variables are set during build time as specified in the Makefile.
var Version string
var Commit string
var Release string

func VersionString(executableName string) string {
	if Version == "" {
		return executableName + " unknown version"
	}

	return fmt.Sprintf("Version: %s\nCommit: %s\nRelease: %s", Version, Commit, Release)
}

func printVersion() {
	fmt.Println(VersionString("gpupgrade"))
}
