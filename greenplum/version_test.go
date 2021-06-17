// Copyright (c) 2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/golang/mock/gomock"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
)

func PostgresGPVersion_5_27_0_beta() {
	fmt.Println("postgres (Greenplum Database) 5.27.0+beta.4 build commit:baef9b9ba885f2f4e4a87d5e201caae969ef4401")
}

func PostgresGPVersion_6_dev() {
	fmt.Println("postgres (Greenplum Database) 6.0.0-beta.1 build dev")
}

func PostgresGPVersion_6_7_1() {
	fmt.Println("postgres (Greenplum Database) 6.7.1 build commit:a21de286045072d8d1df64fa48752b7dfac8c1b7")
}

func PostgresGPVersion_11_341_31() {
	fmt.Println("postgres (Greenplum Database) 11.341.31 build commit:a21de286045072d8d1df64fa48752b7dfac8c1b7")
}

func init() {
	exectest.RegisterMains(
		PostgresGPVersion_5_27_0_beta,
		PostgresGPVersion_6_dev,
		PostgresGPVersion_6_7_1,
		PostgresGPVersion_11_341_31,
	)
	postgresPath = filepath.Join(gphome, "bin", "postgres")
	remotePostgresCmd = fmt.Sprintf(`bash -c "%s --gp-version"`, postgresPath)
}

var postgresPath, remotePostgresCmd string

const gphome = "/usr/local/my-gpdb-home"
const remoteHost = "remote_host"

func TestGPHomeVersion(t *testing.T) {
	cases := []struct {
		name     string
		execMain exectest.Main // the postgres Main implementation to run
		expected string        // the expected semantic version; e.g. "5.1.14"
	}{
		{"handles development versions", PostgresGPVersion_6_dev, "6.0.0"},
		{"handles beta versions", PostgresGPVersion_5_27_0_beta, "5.27.0"},
		{"handles release versions", PostgresGPVersion_6_7_1, "6.7.1"},
		{"handles large versions", PostgresGPVersion_11_341_31, "11.341.31"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			runVersionTest(t, LocalVersion, false, c.execMain, c.expected)
			runVersionTest(t, localFunction, false, c.execMain, c.expected)
			runVersionTest(t, remoteFunction, true, c.execMain, c.expected)
		})
	}

	formatErrorCases := []struct {
		name    string
		version string
	}{
		{"empty string", ""},
		{"only a marker", "(Greenplum Database)"},
	}

	for _, c := range formatErrorCases {
		t.Run(fmt.Sprintf("returns error with %s as input", c.name), func(t *testing.T) {
			_, err := parseVersion(c.version)
			if !errors.Is(err, ErrUnknownVersion) {
				t.Fatalf("returned error %+v, want %+v", err, ErrUnknownVersion)
			}

			// The input should be reflected in any errors, for debugging.
			if !strings.Contains(err.Error(), c.version) {
				t.Errorf("Error() = %q, want it to contain %q", err.Error(), c.version)
			}
		})
	}

	t.Run("bubbles up postgres execution failures", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mock, cleanup := MockExecCommand(ctrl)
		defer cleanup()

		mock.EXPECT().
			Command(postgresPath, []string{"--gp-version"}).
			Return(exectest.Failure)

		_, err := LocalVersion(gphome)

		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Errorf("returned error %#v, want type %T", err, exitErr)
		}
	})
}

func localFunction(ignore string) (semver.Version, error) {
	str, err := NewVersions(gphome).Local()
	if err != nil {
		return semver.Version{}, nil
	}
	return semver.MustParse(str), nil
}

func remoteFunction(ignore string) (semver.Version, error) {
	str, err := NewVersions(gphome).Remote(remoteHost)
	if err != nil {
		return semver.Version{}, nil
	}
	return semver.MustParse(str), nil
}

func runVersionTest(t *testing.T, versionFunc func(string) (semver.Version, error), isRemote bool, execMain exectest.Main, expected string) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock, cleanup := MockExecCommand(ctrl)
	defer cleanup()

	if isRemote {
		mock.EXPECT().
			Command("ssh", []string{"-q", remoteHost, remotePostgresCmd}).
			Return(execMain)
	} else {
		mock.EXPECT().
			Command(postgresPath, []string{"--gp-version"}).
			Return(execMain)
	}

	version, err := versionFunc(gphome)
	if err != nil {
		t.Errorf("returned error: %+v", err)
	}

	expectedVer := semver.MustParse(expected)
	if !version.Equals(expectedVer) {
		t.Errorf("got version %v, want %v", version, expectedVer)
	}
}

func TestGPHomeVersion_OnRemoteHost(t *testing.T) {
	host := "sdw1"

	t.Run("returns remote version using -q to suppress motd banner messages from polluting the version output", func(t *testing.T) {
		execCmd := exectest.NewCommandWithVerifier(PostgresGPVersion_11_341_31, func(cmd string, args ...string) {
			if cmd != "ssh" {
				t.Errorf("got cmd %q want ssh", cmd)
			}

			expected := []string{
				"-q",
				host,
				`bash -c "/usr/local/my-gpdb-home/bin/postgres --gp-version"`,
			}

			if !reflect.DeepEqual(args, expected) {
				t.Errorf("got args %q want %q", args, expected)
			}
		})

		SetExecCommand(execCmd)
		defer ResetExecCommand()

		version, err := NewVersions(gphome).Remote(host)
		if err != nil {
			t.Errorf("unexpected errr %#v", err)
		}

		expected := "11.341.31"
		if version != expected {
			t.Errorf("got version %v, want %v", version, expected)
		}
	})
}
