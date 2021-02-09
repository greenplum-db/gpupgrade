// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands_test

import (
	"testing"

	"github.com/greenplum-db/gpupgrade/cli/commands"
)

func TestVersion(t *testing.T) {
	t.Run("returns empty values when version fields are not set", func(t *testing.T) {
		actual := commands.VersionString("")
		expected := `Version: 
Commit: 
Release: `
		if actual != expected {
			t.Errorf("got version %q want %q", actual, expected)
		}
	})

	t.Run("returns version", func(t *testing.T) {
		commands.Version = "1.2.3"
		commands.Commit = "5889c19"
		commands.Release = "Enterprise"

		actual := commands.VersionString("")
		expected := `Version: 1.2.3
Commit: 5889c19
Release: Enterprise`
		if actual != expected {
			t.Errorf("got version %q want %q", actual, expected)
		}
	})
}

func TestVersionFormat(t *testing.T) {
	commands.Version = "1.2.3"
	commands.Commit = "5889c19"
	commands.Release = "Enterprise"

	cases := []struct {
		format   string
		expected string
	}{
		{
			"multiline",
			`Version: 1.2.3
Commit: 5889c19
Release: Enterprise`,
		}, {
			"oneline",
			`Version: 1.2.3 Commit: 5889c19 Release: Enterprise`,
		},
		{
			"json",
			`{
  "Version": "1.2.3",
  "Commit": "5889c19",
  "Release": "Enterprise"
}`,
		},
		{
			"",
			`Version: 1.2.3
Commit: 5889c19
Release: Enterprise`,
		},
		{
			"non-existent-format",
			`Version: 1.2.3
Commit: 5889c19
Release: Enterprise`,
		},
	}

	for _, c := range cases {
		actual := commands.VersionString(c.format)
		if actual != c.expected {
			t.Errorf("got %q want %q", actual, c.expected)
			t.Logf("got version: %q", actual)
			t.Logf("want version: %q", c.expected)
		}
	}
}
