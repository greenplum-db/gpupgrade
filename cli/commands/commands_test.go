// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"testing"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

func TestGetHubPort(t *testing.T) {
	testlog.SetupLogger()

	t.Run("correctly pulls the port from the stored config", func(t *testing.T) {
		stateDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, stateDir)

		// set GPUPGRADE_HOME to the stateDir to provide a home for the config file
		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
		defer resetEnv()

		// save the expected port value to the conf file
		expected := 12345
		server := hub.New(&hub.Config{Port: expected}, nil, stateDir)
		err := server.SaveConfig()
		if err != nil {
			t.Errorf("got unexpected error %#v", err)
		}

		// looks up port from config file
		port := getHubPort(false)
		if port != expected {
			t.Errorf("got %d expected %d", port, expected)
		}

		// still looks up port from config file whn default port is allowed
		port = getHubPort(true)
		if port != expected {
			t.Errorf("got %d expected %d", port, expected)
		}

	})

	t.Run("uses default port if the config file does not exist", func(t *testing.T) {
		expected := upgrade.DefaultHubPort

		port := getHubPort(true)
		if port != expected {
			t.Errorf("got %d expected %d", port, expected)
		}
	})

}
