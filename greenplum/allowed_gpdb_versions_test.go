//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"errors"
	"testing"

	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/idl"
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
				"5.28.12",
				"5.28.13",
				"5.50.0",
				"6.17.0",
				"6.17.1",
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
				"5.28.0",
				"5.28.11",
				"6.0.0",
				"6.16.9",
				"7.0.0",
			},
			sourceVersionAllowed,
			"sourceVersionAllowed",
			false,
		}, {
			"allowed target versions",
			[]string{
				"6.17.0",
				"6.17.1",
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
				"6.16.0",
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
		localVersion     string
		testLocalVersion func(string) (semver.Version, error)
		expected         error
	}{
		{
			name: "fails when GPDB version has unsupported minor versions",
			localVersion:    semver.MustParse("6.8.0").String(),
			expected: errors.New("source cluster version 6.8.0 is not supported.  The minimum required version is 6.17.0. We recommend the latest version."),
		},
		{
			name: "fails when GPDB version has unsupported major versions",
			localVersion:    semver.MustParse("0.0.0").String(),
			expected: errors.New("source cluster version 0.0.0 is not supported.  The minimum required version is 5.28.12. We recommend the latest version."),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateVersion(c.localVersion, idl.ClusterDestination_SOURCE)
			if err.Error() != c.expected.Error() {
				t.Errorf("got %s want %s", err, c.expected)
			}
		})
	}
}
