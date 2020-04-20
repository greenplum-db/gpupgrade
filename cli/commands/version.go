// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import "fmt"

// This global var UpgradeVersion should have a value set at build time.
// see Makefile for -ldflags "-X etc"
var UpgradeVersion = ""

func VersionString(executableName string) string {
	if UpgradeVersion == "" {
		return executableName + " unknown version"
	}
	return executableName + " version " + UpgradeVersion
}

func printVersion() {
	fmt.Println(VersionString("gpupgrade"))
}
