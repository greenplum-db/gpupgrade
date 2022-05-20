// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"testing"

	"github.com/blang/semver/v4"

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

func PostgresGPVersion_MultiLine() {
	fmt.Println(`/usr/local/greenplum-db-6.18.2/bin/postgres: /usr/local/greenplum-db-5.29.1+dev.1.g0962183f78/lib/libxml2.so.2: no version information available (required by /usr/local/greenplum-db-6.18.2/bin/postgres)
/usr/local/greenplum-db-6.18.2/bin/postgres: /usr/local/greenplum-db-5.29.1+dev.1.g0962183f78/lib/libxml2.so.2: no version information available (required by /usr/local/greenplum-db-6.18.2/bin/postgres)
postgres (Greenplum Database) 6.18.2 build commit:1242aadf0137d3b26ee42c80e579e78bd7a805c7`)
}

func PostgresGPVersion_0_0_0() {
	fmt.Println("postgres (Greenplum Database) 0.0.0 build commit:a21de286045072d8d1df64fa48752b7dfac8c1b7")
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
		PostgresGPVersion_MultiLine,
		PostgresGPVersion_0_0_0,
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
		expected       semver.Version
	}{
		{"handles development versions", PostgresGPVersion_6_dev, semver.MustParse("6.0.0")},
		{"handles beta versions", PostgresGPVersion_5_27_0_beta, semver.MustParse("5.27.0")},
		{"handles release versions", PostgresGPVersion_6_7_1, semver.MustParse("6.7.1")},
		{"handles large versions", PostgresGPVersion_11_341_31, semver.MustParse("11.341.31")},
		{"handles multi line versions", PostgresGPVersion_MultiLine, semver.MustParse("6.18.2")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			SetVersionCommand(exectest.NewCommand(c.versionCommand))
			defer ResetVersionCommand()

			version, err := Version("")
			if err != nil {
				t.Errorf("unexpected error: %+v", err)
			}

			if !version.EQ(c.expected) {
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

			if !reflect.DeepEqual(version, semver.Version{}) {
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
