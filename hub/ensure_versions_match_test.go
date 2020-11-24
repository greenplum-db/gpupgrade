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
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func ResetGpupgradeVersion() {
	hub.GpupgradeVersion = upgrade.GpupgradeVersion
}

func ResetGpupgradeVersionOnHost() {
	hub.GpupgradeVersionOnHost = upgrade.GpupgradeVersionOnHost
}

func TestEnsureGpupgradeVersionsMatch(t *testing.T) {
	testlog.SetupLogger()

	expectedHosts := []string{"sdw1", "sdw2"}
	version_0_3_0 := "Version: 0.3.0 Commit: 35fae54 Release: Dev Build"
	version_0_4_0 := "Version: 0.4.0 Commit: 21b66d7 Release: Dev Build"

	t.Run("ensures gpupgrade versions match on hub and agents", func(t *testing.T) {
		hub.GpupgradeVersion = func() (string, error) {
			return version_0_4_0, nil
		}
		defer ResetGpupgradeVersion()

		var actualHosts []string
		hub.GpupgradeVersionOnHost = func(host string) (string, error) {
			actualHosts = append(actualHosts, host)
			return version_0_4_0, nil
		}
		defer ResetGpupgradeVersionOnHost()

		err := hub.EnsureGpupgradeVersionsMatch(expectedHosts)
		if err != nil {
			t.Errorf("unexpected errr %#v", err)
		}

		sort.Strings(actualHosts)
		if !reflect.DeepEqual(actualHosts, expectedHosts) {
			t.Errorf("got %q want %q", actualHosts, expectedHosts)
		}
	})

	t.Run("errors when failing to get gpupgrade version on the hub", func(t *testing.T) {
		hub.SetExecCommand(exectest.NewCommand(hub.Failure))
		defer hub.ResetExecCommand()

		err := hub.EnsureGpupgradeVersionsMatch(expectedHosts)
		if err == nil {
			t.Errorf("expected an error")
		}
	})

	t.Run("errors when failing to get gpugprade version on the agents", func(t *testing.T) {
		hub.GpupgradeVersion = func() (string, error) {
			return version_0_4_0, nil
		}
		defer ResetGpupgradeVersion()

		var expected error
		errString := "%s: bad agent connection"
		hub.GpupgradeVersionOnHost = func(host string) (string, error) {
			err := fmt.Errorf(errString, host)
			expected = errorlist.Append(expected, err)
			return "", err
		}
		defer ResetGpupgradeVersionOnHost()

		err := hub.EnsureGpupgradeVersionsMatch(expectedHosts)
		var errs errorlist.Errors
		if !errors.As(err, &errs) {
			t.Fatalf("got type %T, want type %T", err, errs)
		}

		if !reflect.DeepEqual(err, expected) {
			t.Fatalf("got err %#v, want %#v", err, expected)
		}
	})

	t.Run("reports version mismatch between hub and agent", func(t *testing.T) {
		hub.GpupgradeVersion = func() (string, error) {
			return version_0_4_0, nil
		}
		defer ResetGpupgradeVersion()

		hub.GpupgradeVersionOnHost = func(host string) (string, error) {
			return version_0_3_0, nil
		}
		defer ResetGpupgradeVersionOnHost()

		err := hub.EnsureGpupgradeVersionsMatch(expectedHosts)
		if err == nil {
			t.Errorf("expected an error")
		}

		expected := hub.MismatchedVersions{version_0_3_0: expectedHosts}
		if strings.HasSuffix(err.Error(), expected.String()) {
			t.Error("expected error to contain mismatched agents")
			t.Logf("got err: %s", err)
			t.Logf("want suffix: %s", expected)
		}
	})
}
