//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"errors"
	"testing"

	"github.com/blang/semver/v4"

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

func TestValidateVersions(t *testing.T) {
	t.Run("passes when given supported versions", func(t *testing.T) {
		gpdbVersion = func(str string) (semver.Version, error) {
			return semver.MustParse("6.50.0"), nil
		}
		defer func() {
			gpdbVersion = LocalVersion
		}()

		err := VerifyCompatibleGPDBVersions("/does/not/matter", "/does/not/matter")
		if err != nil {
			t.Errorf("got unexpected error %#v", err)
		}

	})
}

func TestValidateVersionsErrorCases(t *testing.T) {

	cases := []struct {
		name             string
		testLocalVersion func(string) (semver.Version, error)
		expectedSource   string
		expectedTarget   string
	}{
		{
			"fails when gpdbVersion returns an error",
			func(string) (semver.Version, error) {
				return semver.MustParse("1.2.3"), errors.New("some error")
			},
			"could not determine source cluster version: some error",
			"could not determine target cluster version: some error",
		},
		{
			"fails when sourceVersion and targetVersion have unsupported minor versions",
			func(string) (semver.Version, error) {
				return semver.MustParse("6.8.0"), nil
			},
			"source cluster version 6.8.0 is not supported.  The minimum required version is 6.17.0. We recommend the latest version.",
			"target cluster version 6.8.0 is not supported.  The minimum required version is 6.17.0. We recommend the latest version.",
		},
		{
			"fails when sourceVersion and targetVersion have unsupported major versions",
			func(string) (semver.Version, error) {
				return semver.MustParse("0.0.0"), nil
			},
			"source cluster version 0.0.0 is not supported.  The minimum required version is 5.28.12. We recommend the latest version.",
			"target cluster version 0.0.0 is not supported.  The minimum required version is 6.17.0. We recommend the latest version.",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gpdbVersion = c.testLocalVersion
			defer func() {
				gpdbVersion = LocalVersion
			}()

			err := VerifyCompatibleGPDBVersions("/does/not/matter", "/does/not/matter")

			// make sure both source and target produce an error and that they match
			// the expected error string

			var errs errorlist.Errors
			if !(errors.As(err, &errs)) {
				t.Fatalf("got %T wanted %T", err, errs)
			}
			if len(errs) != 2 {
				t.Fatalf("got %d errors instead of 2", len(errs))
			}

			if errs[0].Error() != c.expectedSource {
				t.Errorf("got %s want %s", errs[0].Error(), c.expectedSource)
			}
			if errs[1].Error() != c.expectedTarget {
				t.Errorf("got %s want %s", errs[1].Error(), c.expectedTarget)
			}
		})
	}
}
