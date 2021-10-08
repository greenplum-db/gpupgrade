//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package upgrade_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

const localVersion = `Version: 1.0.0 Commit: 83aaa4 Release: Enterprise`
const remoteVersion = `Version: 1.1.0 Commit: 63cc21 Release: Enterprise`
const versionStdErr = `
Error: unknown command "\/ersion" for "gpupgrade"
Run 'gpupgrade --help' for usage.
`

func gpupgrade_local_version() {
	fmt.Print(localVersion)
}

func gpupgrade_remote_version() {
	fmt.Print(remoteVersion)
}

func gpupgrade_version_fails() {
	os.Stderr.WriteString("oops")
	os.Exit(1)
}

func init() {
	exectest.RegisterMains(
		gpupgrade_local_version,
		gpupgrade_remote_version,
		gpupgrade_version_fails,
	)
}

func TestGpupgradeVersion(t *testing.T) {
	testlog.SetupLogger()

	t.Run("returns the version", func(t *testing.T) {
		upgrade.SetLocalVersionCommand(exectest.NewCommand(gpupgrade_local_version))
		defer upgrade.ResetLocalVersionCommand()

		version, err := upgrade.LocalVersion()
		if err != nil {
			t.Errorf("unexpected errr %#v", err)
		}

		if version != localVersion {
			t.Errorf("got %q want %q", version, localVersion)
		}
	})

	t.Run("returns error when command fails", func(t *testing.T) {
		upgrade.SetLocalVersionCommand(exectest.NewCommand(gpupgrade_version_fails))
		defer upgrade.ResetLocalVersionCommand()

		version, err := upgrade.LocalVersion()
		var actual *exec.ExitError
		if !errors.As(err, &actual) {
			t.Fatalf("got %#v want ExitError", err)
		}

		if actual.ExitCode() != 1 {
			t.Errorf("got %d want 1 ", actual.ExitCode())
		}

		if strings.HasSuffix(err.Error(), versionStdErr) {
			t.Errorf("got stderr %q want %q", actual.Stderr, versionStdErr)
		}

		if version != "" {
			t.Errorf("got %q want %q", version, "")
		}
	})
}

func TestGpupgradeVersionOnHost(t *testing.T) {
	testlog.SetupLogger()
	host := "sdw1"

	t.Run("returns remote version using -q to suppress motd banner messages from polluting the version output", func(t *testing.T) {

		upgrade.SetRemoteVersionCommand(exectest.NewCommand(gpupgrade_remote_version))
		defer upgrade.ResetRemoteVersionCommand()

		version, err := upgrade.RemoteVersion(host)
		if err != nil {
			t.Errorf("unexpected errr %#v", err)
		}

		if version != remoteVersion {
			t.Errorf("got %q want %q", version, remoteVersion)
		}
	})

	t.Run("returns error when command fails", func(t *testing.T) {
		upgrade.SetRemoteVersionCommand(exectest.NewCommand(gpupgrade_version_fails))
		defer upgrade.ResetRemoteVersionCommand()

		version, err := upgrade.RemoteVersion(host)
		var actual *exec.ExitError
		if !errors.As(err, &actual) {
			t.Fatalf("got %#v want ExitError", err)
		}

		if actual.ExitCode() != 1 {
			t.Errorf("got %d want 1 ", actual.ExitCode())
		}

		if strings.HasSuffix(err.Error(), versionStdErr) {
			t.Errorf("got stderr %q want %q", actual.Stderr, versionStdErr)
		}

		if version != "" {
			t.Errorf("got %q want %q", version, "")
		}
	})
}

func TestEnsureVersionsMatch(t *testing.T) {
	testlog.SetupLogger()

	t.Run("versions match", func(t *testing.T) {
		upgrade.SetLocalVersionCommand(exectest.NewCommand(gpupgrade_local_version))
		defer upgrade.ResetLocalVersionCommand()

		err := upgrade.EnsureGpupgradeVersionsMatch([]string{""})
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("errors when failing to get version on the hub", func(t *testing.T) {
		upgrade.SetLocalVersionCommand(exectest.NewCommand(gpupgrade_version_fails))
		defer upgrade.ResetLocalVersionCommand()

		err := upgrade.EnsureGpupgradeVersionsMatch([]string{""})
		expected := `failed with "oops": exit status 1`
		if !strings.HasSuffix(err.Error(), expected) {
			t.Errorf("got %v want %v", err, expected)
		}
	})

	t.Run("errors when failing to get version on the agents", func(t *testing.T) {
		upgrade.SetLocalVersionCommand(exectest.NewCommand(gpupgrade_local_version))
		defer upgrade.ResetLocalVersionCommand()

		upgrade.SetRemoteVersionCommand(exectest.NewCommand(gpupgrade_version_fails))
		defer upgrade.ResetRemoteVersionCommand()

		hosts := []string{"sdw1", "sdw2"}
		err := upgrade.EnsureGpupgradeVersionsMatch(hosts)
		var expected errorlist.Errors
		if !errors.As(err, &expected) {
			t.Fatalf("got type %T, want type %T", err, expected)
		}

		if !reflect.DeepEqual(err, expected) {
			t.Fatalf("got err %#v, want %#v", err, expected)
		}
	})

	t.Run("errors when hub version does not match agent versions", func(t *testing.T) {
		upgrade.SetLocalVersionCommand(exectest.NewCommand(gpupgrade_local_version))
		defer upgrade.ResetLocalVersionCommand()

		upgrade.SetRemoteVersionCommand(exectest.NewCommand(gpupgrade_remote_version))
		defer upgrade.ResetRemoteVersionCommand()

		hosts := []string{"sdw1"}
		err := upgrade.EnsureGpupgradeVersionsMatch(hosts)
		expected := upgrade.MismatchedVersions{remoteVersion: hosts}
		if !strings.HasSuffix(err.Error(), expected.String()) {
			t.Error("expected error to contain mismatched agents")
			t.Logf("got err: %s", err)
			t.Logf("want suffix: %s", expected)
		}
	})
}
