// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"fmt"

	"github.com/blang/semver/v4"
)

// Change these values to bump the minimum supported versions and associated tests.
const min5xVersion = "5.29.10"
const min6xVersion = "6.24.0"
const min7xVersion = "7.0.0"

var GetSourceVersion = Version
var GetTargetVersion = Version

func VerifyCompatibleGPDBVersions(sourceGPHome, targetGPHome string) error {
	sourceVersion, err := GetSourceVersion(sourceGPHome)
	if err != nil {
		return err
	}

	targetVersion, err := GetTargetVersion(targetGPHome)
	if err != nil {
		return err
	}

	return validate(sourceVersion, targetVersion)
}

func validate(sourceVersion semver.Version, targetVersion semver.Version) error {
	var sourceRange, targetRange semver.Range
	var minSourceVersion, minTargetVersion string

	switch {
	case sourceVersion.Major == 5 && targetVersion.Major == 6:
		sourceRange = semver.MustParseRange(">=" + min5xVersion + " <6.0.0")
		targetRange = semver.MustParseRange(">=" + min6xVersion + " <7.0.0")
		minSourceVersion = min5xVersion
		minTargetVersion = min6xVersion
	case sourceVersion.Major == 6 && targetVersion.Major == 6:
		sourceRange = semver.MustParseRange(">=" + min6xVersion + " <7.0.0")
		targetRange = semver.MustParseRange(">=" + min6xVersion + " <7.0.0")
		minSourceVersion = min6xVersion
		minTargetVersion = min6xVersion
	case sourceVersion.Major == 6 && targetVersion.Major == 7:
		sourceRange = semver.MustParseRange(">=" + min6xVersion + " <7.0.0")
		targetRange = semver.MustParseRange(">=" + min7xVersion + " <8.0.0")
		minSourceVersion = min6xVersion
		minTargetVersion = min7xVersion
	case sourceVersion.Major == 7 && targetVersion.Major == 7:
		sourceRange = semver.MustParseRange(">=" + min7xVersion + " <8.0.0")
		targetRange = semver.MustParseRange(">=" + min7xVersion + " <8.0.0")
		minSourceVersion = min7xVersion
		minTargetVersion = min7xVersion
	default:
		return fmt.Errorf("Unsupported source and target versions. "+
			"Found source version %s and target version %s. "+
			"Upgrade is only supported for Greenplum 5 to 6 and Greenplum 6 to 7. "+
			"Check the documentation for further information.", sourceVersion, targetVersion)
	}

	if !sourceRange(sourceVersion) {
		return fmt.Errorf("Source cluster version %s is not supported. "+
			"The minimum required version is %s. "+
			"We recommend the latest version.", sourceVersion, minSourceVersion)
	}

	if !targetRange(targetVersion) {
		return fmt.Errorf("Target cluster version %s is not supported. "+
			"The minimum required version is %s. "+
			"We recommend the latest version.", targetVersion, minTargetVersion)
	}

	return nil
}
