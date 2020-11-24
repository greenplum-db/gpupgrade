//  Copyright (c) 2017-2020 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package upgrade_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

const version_0_4_0 = "Version: 0.4.0 Commit: 21b66d7 Release: Dev Build"
const versionStdErr = `
Error: unknown command "\/ersion" for "gpupgrade"
Run 'gpupgrade --help' for usage.
`

func gpupgradeVersion() {
	os.Stdout.WriteString(version_0_4_0)
}

func gpupgradeVersion_Errors() {
	os.Stderr.WriteString(versionStdErr)
	os.Exit(1)
}

func init() {
	exectest.RegisterMains(
		gpupgradeVersion,
		gpupgradeVersion_Errors,
	)
}

func TestGpupgradeVersion(t *testing.T) {
	testlog.SetupLogger()

	t.Run("returns the version", func(t *testing.T) {
		execCmd := exectest.NewCommandWithVerifier(gpupgradeVersion, func(cmd string, args ...string) {
			expected := filepath.Join(testutils.MustGetExecutablePath(t), "gpupgrade")
			if cmd != expected {
				t.Errorf("got cmd %q want %q", cmd, expected)
			}

			expectedArgs := []string{"version", "--format", "oneline"}
			if !reflect.DeepEqual(args, expectedArgs) {
				t.Errorf("got args %q want %q", args, expectedArgs)
			}
		})

		upgrade.SetExecCommand(execCmd)
		defer upgrade.ResetExecCommand()

		version, err := upgrade.GpupgradeVersion()
		if err != nil {
			t.Errorf("unexpected errr %#v", err)
		}

		if version != version_0_4_0 {
			t.Errorf("got %q want %q", version, version_0_4_0)
		}
	})

	t.Run("returns error when command fails", func(t *testing.T) {
		upgrade.SetExecCommand(exectest.NewCommand(gpupgradeVersion_Errors))
		defer upgrade.ResetExecCommand()

		version, err := upgrade.GpupgradeVersion()
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

	t.Run("returns the version", func(t *testing.T) {
		execCmd := exectest.NewCommandWithVerifier(gpupgradeVersion, func(cmd string, args ...string) {
			if cmd != "ssh" {
				t.Errorf("got cmd %q want ssh", cmd)
			}

			expected := []string{
				host,
				fmt.Sprintf(`bash -c "%s/gpupgrade version --format oneline"`, testutils.MustGetExecutablePath(t)),
			}

			if !reflect.DeepEqual(args, expected) {
				t.Errorf("got args %q want %q", args, expected)
			}
		})

		upgrade.SetExecCommand(execCmd)
		defer upgrade.ResetExecCommand()

		version, err := upgrade.GpupgradeVersionOnHost(host)
		if err != nil {
			t.Errorf("unexpected errr %#v", err)
		}

		if version != version_0_4_0 {
			t.Errorf("got %q want %q", version, version_0_4_0)
		}
	})

	t.Run("returns error when command fails", func(t *testing.T) {
		upgrade.SetExecCommand(exectest.NewCommand(gpupgradeVersion_Errors))
		defer upgrade.ResetExecCommand()

		version, err := upgrade.GpupgradeVersionOnHost(host)
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
