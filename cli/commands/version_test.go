// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands_test

import (
	"testing"

	"github.com/greenplum-db/gpupgrade/cli/commands"
)

func TestVersionString(t *testing.T) {
	t.Run("returns unknown version when version is not set", func(t *testing.T) {
		commands.UpgradeVersion = ""

		actual := commands.VersionString("gpupgrade")
		expected := "gpupgrade unknown version"
		if actual != expected {
			t.Errorf("got version %q want %q", actual, expected)
		}
	})

	t.Run("returns version", func(t *testing.T) {
		commands.UpgradeVersion = "1.2.3"

		actual := commands.VersionString("gpupgrade")
		expected := "gpupgrade version 1.2.3"
		if actual != expected {
			t.Errorf("got version %q want %q", actual, expected)
		}
	})
}
