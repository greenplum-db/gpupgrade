// Copyright (c) 2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
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

func EmptyString() {
	fmt.Println("")
}

func MarkerOnly() {
	fmt.Println("postgres (Greenplum Database)")
}

func FailedMain() {
	os.Exit(1)
}

func init() {
	exectest.RegisterMains(
		PostgresGPVersion_5_27_0_beta,
		PostgresGPVersion_6_dev,
		PostgresGPVersion_6_7_1,
		PostgresGPVersion_11_341_31,
		EmptyString,
		MarkerOnly,
		FailedMain,
	)
}

func TestVersion_Parsing(t *testing.T) {
	testlog.SetupLogger()

	cases := []struct {
		name           string
		versionCommand exectest.Main
		expected       string
	}{
		{name: "handles development versions", versionCommand: PostgresGPVersion_6_dev, expected: "6.0.0"},
		{name: "handles beta versions", versionCommand: PostgresGPVersion_5_27_0_beta, expected: "5.27.0"},
		{name: "handles release versions", versionCommand: PostgresGPVersion_6_7_1, expected: "6.7.1"},
		{name: "handles large versions", versionCommand: PostgresGPVersion_11_341_31, expected: "11.341.31"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			SetVersionCommand(exectest.NewCommand(c.versionCommand))
			defer ResetVersionCommand()

			version, err := Version("")
			if err != nil {
				t.Errorf("unexpected error: %+v", err)
			}

			if version != c.expected {
				t.Errorf("got version %v, want %v", version, c.expected)
			}
		})
	}

	errCases := []struct {
		name           string
		versionCommand exectest.Main
		expected       error
	}{
		{name: "handles empty version", versionCommand: EmptyString, expected: errors.New(`Greenplum version "\n" is not of the form "postgres (Greenplum Database) #.#.#"`)},
		{name: "handles only marker string", versionCommand: MarkerOnly, expected: errors.New(`Greenplum version "postgres (Greenplum Database)\n" is not of the form "postgres (Greenplum Database) #.#.#"`)},
	}

	for _, c := range errCases {
		t.Run(c.name, func(t *testing.T) {
			SetVersionCommand(exectest.NewCommand(c.versionCommand))
			defer ResetVersionCommand()

			version, err := Version("")
			if err.Error() != c.expected.Error() {
				t.Errorf("got %q want %q", err, c.expected)
			}

			if version != "" {
				t.Errorf("unexpected version %q", version)
			}
		})
	}

	t.Run("returns postgres execution failures", func(t *testing.T) {
		SetVersionCommand(exectest.NewCommand(FailedMain))
		defer ResetVersionCommand()

		_, err := Version("")
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Errorf("returned error %#v, want type %T", err, exitErr)
		}
	})
}
