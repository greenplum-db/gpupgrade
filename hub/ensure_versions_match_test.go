// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

func ResetGpupgradeVersion() {
	hub.GpupgradeVersion = upgrade.GpupgradeVersion
}

func TestValidateGpupgradeVersion(t *testing.T) {
	testlog.SetupLogger()

	agentHosts := []string{"sdw1", "sdw2"}hubHost := "mdw"

	expectedHosts := append(agentHosts, hubHost)
	sort.Strings(expectedHosts)
	version_0_3_0 := "Version: 0.3.0 Commit: 35fae54 Release: Dev Build"
	version_0_4_0 := "Version: 0.4.0 Commit: 21b66d7 Release: Dev Build"

	agentError := errors.New("sdw2: bad agent connection")

	t.Run("EnsureGpupgradeAndGPDBVersionsMatch successfully requests the version of gpupgrade on hub and agents", func(t *testing.T) {
		var actualHosts []string
		hub.GpupgradeVersion = func(host string) (string, error) {
			actualHosts = append(actualHosts, host)
			return "6.0.0", nil
		}

		err := hub.EnsureGpupgradeAndGPDBVersionsMatch(agentHosts, hubHost)
		if err != nil {
			t.Errorf("unexpected errr %#v", err)
		}

		sort.Strings(actualHosts)
		if !reflect.DeepEqual(actualHosts, expectedHosts) {
			t.Errorf("got %q want %q", actualHosts, expectedHosts)
		}
	})

	t.Run("matches version information from host and agents", func(t *testing.T) {
		hub.GpupgradeVersion = func(host string) (string, error) {
			return version_0_4_0, nil
		}
		defer ResetGpupgradeVersion()

		err := hub.EnsureGpupgradeAndGPDBVersionsMatch(agentHosts, hubHost)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("errors when failing to get gpupgrade version on the hub", func(t *testing.T) {
		hub.SetExecCommand(exectest.NewCommand(hub.Failure))
		defer hub.ResetExecCommand()

		err := hub.EnsureGpupgradeAndGPDBVersionsMatch(agentHosts, hubHost)
		if err == nil {
			t.Errorf("expected an error")
		}
	})

	t.Run("errors when failing to get gpugprade version on the agents", func(t *testing.T) {
		hub.GpupgradeVersion = func(host string) (string, error) {
			if host != agentHosts[1] {
				return version_0_4_0, nil
			}
			return "", agentError
		}
		defer ResetGpupgradeVersion()

		err := hub.EnsureGpupgradeAndGPDBVersionsMatch(agentHosts, hubHost)
		if err == nil {
			t.Errorf("expected an error")
		}

		if !errors.Is(err, agentError) {
			t.Errorf("expected %v, got %v", agentError, err)
		}
	})

	t.Run("reports version mismatch between hub and agent", func(t *testing.T) {
		hub.GpupgradeVersion = func(host string) (string, error) {
			if host == hubHost {
				return version_0_4_0, nil
			}
			return version_0_3_0, nil
		}
		defer ResetGpupgradeVersion()

		err := hub.EnsureGpupgradeAndGPDBVersionsMatch(agentHosts, hubHost)
		if err == nil {
			t.Errorf("expected an error")
		}

		expected := hub.MismatchedVersions{version_0_3_0: agentHosts}
		if strings.HasSuffix(err.Error(), expected.String()) {
			t.Error("expected error to contain mismatched agents")
			t.Logf("got err: %s", err)
			t.Logf("want suffix: %s", expected)
		}
	})
}
