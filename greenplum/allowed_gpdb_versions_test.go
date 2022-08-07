// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
)

func TestVerifyCompatibleGPDBVersions(t *testing.T) {
	t.Run("validates source and target cluster versions", func(t *testing.T) {
		SetVersionCommand(exectest.NewCommand(PostgresGPVersion_6_99_0))
		defer ResetVersionCommand()

		err := VerifyCompatibleGPDBVersions("/usr/local/greenplum-db-source", "/usr/local/greenplum-db-target")
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("errors when failing to get source cluster version", func(t *testing.T) {
		expected := os.ErrNotExist
		GetSourceVersion = func(gphome string) (semver.Version, error) {
			return semver.Version{}, expected
		}
		defer func() {
			GetSourceVersion = Version
		}()

		err := VerifyCompatibleGPDBVersions("", "")
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", expected, err)
		}
	})

	t.Run("errors when failing to get target cluster version", func(t *testing.T) {
		GetSourceVersion = func(gphome string) (semver.Version, error) {
			return semver.Version{}, nil
		}
		defer func() {
			GetSourceVersion = Version
		}()

		expected := os.ErrNotExist
		GetTargetVersion = func(gphome string) (semver.Version, error) {
			return semver.Version{}, expected
		}
		defer func() {
			GetTargetVersion = Version
		}()

		err := VerifyCompatibleGPDBVersions("", "")
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", expected, err)
		}
	})

	t.Run("errors when failing to validate cluster versions", func(t *testing.T) {
		SetVersionCommand(exectest.NewCommand(PostgresGPVersion_11_341_31))
		defer ResetVersionCommand()

		err := VerifyCompatibleGPDBVersions("", "")
		expected := "Unsupported source and target versions. " +
			"Found source version 11.341.31 and target version 11.341.31. "
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("expected error %+v to contain %q", err.Error(), expected)
		}
	})
}

func TestValidate(t *testing.T) {
	min5xVersionIncrementedMinor := MustIncrementMinor(t, min5xVersion)
	min5xVersionIncrementedPatch := MustIncrementPatch(t, min5xVersion)

	min6xVersionIncrementedMinor := MustIncrementMinor(t, min6xVersion)
	min6xVersionIncrementedPatch := MustIncrementPatch(t, min6xVersion)

	min7xVersionIncrementedMinor := MustIncrementMinor(t, min7xVersion)
	min7xVersionIncrementedPatch := MustIncrementPatch(t, min7xVersion)

	t.Run("validates cluster versions", func(t *testing.T) {
		cases := []struct {
			name          string
			sourceVersion semver.Version
			targetVersion semver.Version
		}{
			// 5->6 allowed version tests
			{
				name:          "source and target versions are exactly the minimum supported versions",
				sourceVersion: semver.MustParse(min5xVersion),
				targetVersion: semver.MustParse(min6xVersion),
			},
			{
				name:          "source version meets the minimum minor version",
				sourceVersion: min5xVersionIncrementedMinor,
				targetVersion: semver.MustParse(min6xVersion),
			},
			{
				name:          "source version meets the minimum patch version",
				sourceVersion: min5xVersionIncrementedPatch,
				targetVersion: semver.MustParse(min6xVersion),
			},
			{
				name:          "target version meets the minimum minor version",
				sourceVersion: semver.MustParse(min5xVersion),
				targetVersion: min6xVersionIncrementedMinor,
			},
			{
				name:          "target version meets the minimum patch version",
				sourceVersion: semver.MustParse(min5xVersion),
				targetVersion: min6xVersionIncrementedPatch,
			},
			// 6->6 allowed version tests
			{
				name:          "source and target versions are exactly the minimum supported versions",
				sourceVersion: semver.MustParse(min6xVersion),
				targetVersion: semver.MustParse(min6xVersion),
			},
			{
				name:          "source version meets the minimum minor version",
				sourceVersion: min6xVersionIncrementedMinor,
				targetVersion: semver.MustParse(min6xVersion),
			},
			{
				name:          "source version meets the minimum patch version",
				sourceVersion: min6xVersionIncrementedPatch,
				targetVersion: semver.MustParse(min6xVersion),
			},
			{
				name:          "target version meets the minimum minor version",
				sourceVersion: semver.MustParse(min6xVersion),
				targetVersion: min6xVersionIncrementedMinor,
			},
			{
				name:          "target version meets the minimum patch version",
				sourceVersion: semver.MustParse(min6xVersion),
				targetVersion: min6xVersionIncrementedPatch,
			},
			// 6->7 allowed version tests
			{
				name:          "source and target versions are exactly the minimum supported versions",
				sourceVersion: semver.MustParse(min6xVersion),
				targetVersion: semver.MustParse(min7xVersion),
			},
			{
				name:          "source version meets the minimum minor version",
				sourceVersion: min6xVersionIncrementedMinor,
				targetVersion: semver.MustParse(min7xVersion),
			},
			{
				name:          "source version meets the minimum patch version",
				sourceVersion: min6xVersionIncrementedPatch,
				targetVersion: semver.MustParse(min7xVersion),
			},
			{
				name:          "target version meets the minimum minor version",
				sourceVersion: semver.MustParse(min6xVersion),
				targetVersion: min7xVersionIncrementedMinor,
			},
			{
				name:          "target version meets the minimum patch version",
				sourceVersion: semver.MustParse(min6xVersion),
				targetVersion: min7xVersionIncrementedPatch,
			},
			// 7->7 allowed version tests
			{
				name:          "source and target versions are exactly the minimum supported versions",
				sourceVersion: semver.MustParse(min7xVersion),
				targetVersion: semver.MustParse(min7xVersion),
			},
			{
				name:          "source version meets the minimum minor version",
				sourceVersion: min7xVersionIncrementedMinor,
				targetVersion: semver.MustParse(min7xVersion),
			},
			{
				name:          "source version meets the minimum patch version",
				sourceVersion: min7xVersionIncrementedPatch,
				targetVersion: semver.MustParse(min7xVersion),
			},
			{
				name:          "target version meets the minimum minor version",
				sourceVersion: semver.MustParse(min7xVersion),
				targetVersion: min7xVersionIncrementedMinor,
			},
			{
				name:          "target version meets the minimum patch version",
				sourceVersion: semver.MustParse(min7xVersion),
				targetVersion: min7xVersionIncrementedPatch,
			},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				err := validate(c.sourceVersion, c.targetVersion)
				if err != nil {
					t.Errorf("unexpected err %#v", err)
				}
			})
		}
	})

	t.Run("errors for invalid cluster versions", func(t *testing.T) {
		min5xVersionDecrementedMinor := MustDecrementMinor(t, min5xVersion)
		min5xVersionDecrementedPatch := MustDecrementPatch(t, min5xVersion)

		min6xVersionDecrementedMinor := MustDecrementMinor(t, min6xVersion)
		min6xVersionDecrementedPatch := MustDecrementPatch(t, min6xVersion)

		// NOTE: uncomment once we bump min7xVersion to be above 7.0.0
		//min7xVersionDecrementedMinor := MustDecrementMinor(t, min7xVersion)
		//min7xVersionDecrementedPatch := MustDecrementPatch(t, min7xVersion)

		errorCases := []struct {
			name          string
			sourceVersion semver.Version
			targetVersion semver.Version
			toContain     string
		}{
			// generic disallowed version tests
			{
				name:          "source version is not supported",
				sourceVersion: semver.MustParse("4.99.99"),
				targetVersion: semver.MustParse(min6xVersion),
				toContain:     "source version 4.99.99",
			},
			{
				name:          "target version is not supported",
				sourceVersion: semver.MustParse(min6xVersion),
				targetVersion: semver.MustParse("50.0.0"),
				toContain:     "target version 50.0.0",
			},
			// 5->6 disallowed version tests
			{
				name:          "source version does not meet the minimum minor version",
				sourceVersion: min5xVersionDecrementedMinor,
				targetVersion: semver.MustParse(min6xVersion),
				toContain:     fmt.Sprintf("Source cluster version %s is not supported", min5xVersionDecrementedMinor),
			},
			{
				name:          "source version does not meet the minimum patch version",
				sourceVersion: min5xVersionDecrementedPatch,
				targetVersion: semver.MustParse(min6xVersion),
				toContain:     fmt.Sprintf("Source cluster version %s is not supported", min5xVersionDecrementedPatch),
			},
			{
				name:          "target version does not meet the minimum minor version",
				sourceVersion: semver.MustParse(min5xVersion),
				targetVersion: min6xVersionDecrementedMinor,
				toContain:     fmt.Sprintf("Target cluster version %s is not supported", min6xVersionDecrementedMinor),
			},
			{
				name:          "target version does not meet the minimum patch version",
				sourceVersion: semver.MustParse(min5xVersion),
				targetVersion: min6xVersionDecrementedPatch,
				toContain:     fmt.Sprintf("Target cluster version %s is not supported", min6xVersionDecrementedPatch),
			},
			// 6->6 disallowed version tests
			{
				name:          "source version does not meet the minimum minor version",
				sourceVersion: min6xVersionDecrementedMinor,
				targetVersion: semver.MustParse(min6xVersion),
				toContain:     fmt.Sprintf("Source cluster version %s is not supported", min6xVersionDecrementedMinor),
			},
			{
				name:          "source version does not meet the minimum patch version",
				sourceVersion: min6xVersionDecrementedPatch,
				targetVersion: semver.MustParse(min6xVersion),
				toContain:     fmt.Sprintf("Source cluster version %s is not supported", min6xVersionDecrementedPatch),
			},
			{
				name:          "target version does not meet the minimum minor version",
				sourceVersion: semver.MustParse(min6xVersion),
				targetVersion: min6xVersionDecrementedMinor,
				toContain:     fmt.Sprintf("Target cluster version %s is not supported", min6xVersionDecrementedMinor),
			},
			{
				name:          "target version does not meet the minimum patch version",
				sourceVersion: semver.MustParse(min6xVersion),
				targetVersion: min6xVersionDecrementedPatch,
				toContain:     fmt.Sprintf("Target cluster version %s is not supported", min6xVersionDecrementedPatch),
			},
			// 6->7 disallowed version tests
			{
				name:          "source version does not meet the minimum minor version",
				sourceVersion: min6xVersionDecrementedMinor,
				targetVersion: semver.MustParse(min7xVersion),
				toContain:     fmt.Sprintf("Source cluster version %s is not supported", min6xVersionDecrementedMinor),
			},
			{
				name:          "source version does not meet the minimum patch version",
				sourceVersion: min6xVersionDecrementedPatch,
				targetVersion: semver.MustParse(min7xVersion),
				toContain:     fmt.Sprintf("Source cluster version %s is not supported", min6xVersionDecrementedPatch),
			},
			// =========================================================================================================
			// NOTE: uncomment once we bump min7xVersion to be above 7.0.0
			// =========================================================================================================
			//{
			//	name:          "target version does not meet the minimum minor version",
			//	sourceVersion: semver.MustParse(min6xVersion),
			//	targetVersion: min7xVersionDecrementedMinor,
			//	toContain:     fmt.Sprintf("Target cluster version %s is not supported", min7xVersionDecrementedMinor),
			//},
			//{
			//	name:          "target version does not meet the minimum patch version",
			//	sourceVersion: semver.MustParse(min6xVersion),
			//	targetVersion: min7xVersionDecrementedPatch,
			//	toContain:     fmt.Sprintf("Target cluster version %s is not supported", min7xVersionDecrementedPatch),
			//},
			//
			//// 7->7 disallowed version tests
			//{
			//	name:          "source version does not meet the minimum minor version",
			//	sourceVersion: min7xVersionDecrementedMinor,
			//	targetVersion: semver.MustParse(min7xVersion),
			//	toContain:     fmt.Sprintf("Source cluster version %s is not supported", min7xVersionDecrementedMinor),
			//},
			//{
			//	name:          "source version does not meet the minimum patch version",
			//	sourceVersion: min7xVersionDecrementedPatch,
			//	targetVersion: semver.MustParse(min7xVersion),
			//	toContain:     fmt.Sprintf("Source cluster version %s is not supported", min7xVersionDecrementedPatch),
			//},
			//{
			//	name:          "target version does not meet the minimum minor version",
			//	sourceVersion: semver.MustParse(min7xVersion),
			//	targetVersion: min7xVersionDecrementedPatch,
			//	toContain:     fmt.Sprintf("Target cluster version %s is not supported", min7xVersionDecrementedPatch),
			//},
			//{
			//	name:          "target version does not meet the minimum patch version",
			//	sourceVersion: semver.MustParse(min7xVersion),
			//	targetVersion: min7xVersionDecrementedPatch,
			//	toContain:     fmt.Sprintf("Target cluster version %s is not supported", min7xVersionDecrementedPatch),
			//},
		}

		for _, c := range errorCases {
			t.Run(c.name, func(t *testing.T) {
				err := validate(c.sourceVersion, c.targetVersion)
				if err == nil {
					t.Error("expected error got nil")
				}

				if !strings.Contains(err.Error(), c.toContain) {
					t.Errorf("expected error: %+v", err.Error())
					t.Errorf("to contain: %q", c.toContain)
				}
			})
		}
	})
}

func MustIncrementMinor(t *testing.T, version string) semver.Version {
	semverVersion := semver.MustParse(version)
	err := semverVersion.IncrementMinor()
	if err != nil {
		t.Fatalf("failed to increment minor for version %q: %v", version, err)
	}

	return semverVersion
}

func MustIncrementPatch(t *testing.T, version string) semver.Version {
	semverVersion := semver.MustParse(version)
	err := semverVersion.IncrementPatch()
	if err != nil {
		t.Fatalf("failed to increment patch for version %q: %v", version, err)
	}

	return semverVersion
}

func MustDecrementMinor(t *testing.T, version string) semver.Version {
	semverVersion := semver.MustParse(version)
	semverVersion.Minor--
	semverVersion.Patch = 0
	return semverVersion
}

func MustDecrementPatch(t *testing.T, version string) semver.Version {
	semverVersion := semver.MustParse(version)
	if semverVersion.Patch == 0 {
		semverVersion.Minor--
		return semverVersion
	}

	semverVersion.Patch--
	return semverVersion
}
