// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
)

// These variables are set during build time as specified in the Makefile.
var Version string
var Commit string
var Release string

func VersionString(format string) string {
	const oneline = `Version: %s Commit: %s Release: %s`

	const json = `{
  "Version": %q,
  "Commit": %q,
  "Release": %q
}`

	const multiline = `Version: %s
Commit: %s
Release: %s`

	switch format {
	case "oneline":
		return fmt.Sprintf(oneline, Version, Commit, Release)
	case "json":
		return fmt.Sprintf(json, Version, Commit, Release)
	}

	return fmt.Sprintf(multiline, Version, Commit, Release)
}

func printVersion(format string) {
	fmt.Println(VersionString(format))
}
