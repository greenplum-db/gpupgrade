// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
)

func gpupgradeVersion() {}

func init() {
	exectest.RegisterMains(
		gpupgradeVersion,
	)
}

func ResetGetVersion() {
	hub.GetGpupgradeVersionFunc = hub.GetGpupgradeVersion
}

func TestValidateGpupgradeVersion(t *testing.T) {
	testlog.SetupLogger()

	agentHosts := []string{"sdw1", "sdw2"}
	hubHost := "mdw"

	version_0_3_0 := "Version: 0.3.0 Commit: 35fae54 Release: Dev Build"
	version_0_4_0 := "Version: 0.4.0 Commit: 21b66d7 Release: Dev Build"

	agentError := errors.New("sdw2: bad agent connection")

	t.Run("EnsureGpupgradeAndGPDBVersionsMatch successfully requests the version of gpupgrade on hub and agents", func(t *testing.T) {
		var expectedArgs []string
		for _, host := range append(agentHosts, hubHost) {
			expectedArgs = append(expectedArgs, fmt.Sprintf(`%s bash -c "%s/gpupgrade version"`, host, mustGetExecutablePath(t)))
		}

		var actualArgs []string
		execCmd := exectest.NewCommandWithVerifier(gpupgradeVersion, func(name string, args ...string) {
			if name != "ssh" {
				t.Errorf("execCommand got %q want ssh", name)
			}

			actualArgs = append(actualArgs, strings.Join(args, " "))
		})

		hub.SetExecCommand(execCmd)
		defer hub.ResetExecCommand()

		err := hub.EnsureGpupgradeAndGPDBVersionsMatch(agentHosts, hubHost)
		if err != nil {
			t.Errorf("unexpected errr %#v", err)
		}

		sort.Strings(actualArgs)
		sort.Strings(expectedArgs)
		if !reflect.DeepEqual(actualArgs, expectedArgs) {
			t.Errorf("got %q, want %q", actualArgs, expectedArgs)
		}
	})

	t.Run("matches version information from host and agents", func(t *testing.T) {
		hub.GetGpupgradeVersionFunc = func(host string) (string, error) {
			return version_0_4_0, nil
		}
		defer ResetGetVersion()

		err := hub.EnsureGpupgradeAndGPDBVersionsMatch(agentHosts, hubHost)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("errors when execCommand fails", func(t *testing.T) {
		hub.SetExecCommand(exectest.NewCommand(hub.Failure))
		defer hub.ResetExecCommand()

		err := hub.EnsureGpupgradeAndGPDBVersionsMatch(agentHosts, hubHost)
		if err == nil {
			t.Errorf("expected an error")
		}
	})

	t.Run("errors when agent cannot retrieve version information", func(t *testing.T) {
		hub.GetGpupgradeVersionFunc = func(host string) (string, error) {
			if host != agentHosts[1] {
				return version_0_4_0, nil
			}
			return "", agentError
		}
		defer ResetGetVersion()

		err := hub.EnsureGpupgradeAndGPDBVersionsMatch(agentHosts, hubHost)
		if err == nil {
			t.Errorf("expected an error")
		}

		if !errors.Is(err, agentError) {
			t.Errorf("expected %v, got %v", agentError, err)
		}
	})

	t.Run("reports version mismatch between hub and agent", func(t *testing.T) {
		hub.GetGpupgradeVersionFunc = func(host string) (string, error) {
			if host == hubHost {
				return version_0_4_0, nil
			}
			return version_0_3_0, nil
		}
		defer ResetGetVersion()

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
