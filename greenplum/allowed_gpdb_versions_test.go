// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func TestAllowedVersions(t *testing.T) {
	cases := []struct {
		name          string
		versions      []string
		validator     semver.Range
		validatorName string
		expected      bool
	}{
		{
			"allowed source versions",
			[]string{
				"5.29.1",
				"5.29.13",
				"5.50.0",
				"6.18.0",
				"6.18.1",
				"6.50.1",
			},
			sourceVersionAllowed,
			"sourceVersionAllowed",
			true,
		}, {
			"disallowed source versions",
			[]string{
				"4.3.0",
				"5.0.0",
				"5.28.11",
				"5.29.0",
				"6.0.0",
				"6.17.9",
				"7.0.0",
			},
			sourceVersionAllowed,
			"sourceVersionAllowed",
			false,
		}, {
			"allowed target versions",
			[]string{
				"6.18.0",
				"6.18.1",
				"6.50.1",
			},
			targetVersionAllowed,
			"targetVersionAllowed",
			true,
		}, {
			"disallowed target versions",
			[]string{
				"4.3.0",
				"5.0.0",
				"5.27.0",
				"5.28.0",
				"5.50.0",
				"6.0.0",
				"6.17.0",
				"7.0.0",
			},
			targetVersionAllowed,
			"targetVersionAllowed",
			false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, v := range c.versions {
				ver := semver.MustParse(v)
				actual := c.validator(ver)

				if actual != c.expected {
					t.Errorf("%s(%q) = %t, want %t", c.validatorName, v, actual, c.expected)
				}
			}
		})
	}
}

func TestValidateVersionsErrorCases(t *testing.T) {
	cases := []struct {
		name             string
		localVersion     semver.Version
		testLocalVersion func(string) (semver.Version, error)
		expected         error
	}{
		{
			name:         "fails when GPDB version has unsupported minor versions",
			localVersion: semver.MustParse("6.8.0"),
			expected:     errors.New("source cluster version 6.8.0 is not supported.  The minimum required version is 6.18.0. We recommend the latest version."),
		},
		{
			name:         "fails when GPDB version has unsupported major versions",
			localVersion: semver.MustParse("0.0.0"),
			expected:     errors.New("source cluster version 0.0.0 is not supported.  The minimum required version is 5.29.1. We recommend the latest version."),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateVersion(c.localVersion, idl.ClusterDestination_source)
			if err.Error() != c.expected.Error() {
				t.Errorf("got %s want %s", err, c.expected)
			}
		})
	}
}

func TestVerifyCompatibleGPDBVersions(t *testing.T) {
	testlog.SetupLogger()

	t.Run("returns error when gphome is incorrect", func(t *testing.T) {
		err := VerifyCompatibleGPDBVersions("/usr/local/greenplum-db-source-typo", "")
		var pathError *os.PathError
		if !errors.As(err, &pathError) {
			t.Errorf("got type %T want %T", err, pathError)
		}
	})

	t.Run("returns combined errors when source and target cluster versions are invalid", func(t *testing.T) {
		SetVersionCommand(exectest.NewCommand(PostgresGPVersion_0_0_0))
		defer ResetVersionCommand()

		err := VerifyCompatibleGPDBVersions("", "")
		var errs errorlist.Errors
		if !errors.As(err, &errs) {
			t.Fatalf("got error %#v, want type %T", err, errs)
		}

		if len(errs) != 2 {
			t.Errorf("got %d errors want 2", len(errs))
		}

		expected := "source cluster version 0.0.0 is not supported"
		if !strings.Contains(errs[0].Error(), expected) {
			t.Errorf("expected error %+v to contain %q", errs[0], expected)
		}

		expected = "target cluster version 0.0.0 is not supported"
		if !strings.Contains(errs[1].Error(), expected) {
			t.Errorf("expected error %+v to contain %q", errs[1], expected)
		}
	})
}
