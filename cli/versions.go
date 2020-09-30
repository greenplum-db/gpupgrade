// Copyright (c) 2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"

	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

// Note that we represent the source and target versions separately.  Another
// option is a matrix explicitly listing supported source/target combinations.
// However, pg_upgrade supports upgrade from any version to any version.
// We are not sure yet if we are doing that for gpupgrade.

var (
	// sourceVersionAllowed returns whether or not the given semver.Version is a
	// valid source GPDB cluster version.
	sourceVersionAllowed semver.Range

	// targetVersionAllowed returns whether or not the given semver.Version is a
	// valid target GPDB cluster version.
	targetVersionAllowed semver.Range
)

// Source and Target Versions: modify these lists to control what will be allowed
// by the utility.  Map entries are of the form: GPDB_VERSION : MIN_ALLOWED_SEMVER

var minSourceVersions = map[int]string{
	5: "5.28.0",
	6: "6.9.0",
}

var minTargetVersions = map[int]string{
	6: "6.9.0",
}

// The below boilerplate turns the source/targetRanges variables into
// source/targetVersionAllowed. You shouldn't need to touch it.

func init() {
	accumulateRanges(&sourceVersionAllowed, minSourceVersions)
	accumulateRanges(&targetVersionAllowed, minTargetVersions)
}

func accumulateRanges(a *semver.Range, minVersions map[int]string) {
	for v, min := range minVersions {
		// for example, 5: "5.28.0" becomes the Range string ">=5.28.0 <6.0.0"
		str := fmt.Sprintf(">=%s <%d.0.0", min, v+1)
		r := semver.MustParseRange(str)

		if *a == nil {
			*a = r
		} else {
			*a = a.OR(r)
		}
	}
}

func minSourceVersion() string {
	var min int
	for major := range minSourceVersions {
		if min == 0 {
			min = major
		}
		if major < min {
			min = major
		}
	}
	return semver.MustParse(minSourceVersions[min]).String()
}

var gpHomeVersion = greenplum.GPHomeVersion

func ValidateVersions(sourceGPHome, targetGPHome string) error {
	var err error

	vErr := validateVersion(sourceGPHome, "source")
	err = errorlist.Append(err, vErr)

	vErr = validateVersion(targetGPHome, "target")
	err = errorlist.Append(err, vErr)

	return err
}

func validateVersion(gpHome string, context string) error {
	versionsAllowed := sourceVersionAllowed
	minVersions := minSourceVersions
	if context == "target" {
		versionsAllowed = targetVersionAllowed
		minVersions = minTargetVersions
	}

	version, err := gpHomeVersion(gpHome)
	if err == nil && !versionsAllowed(version) {
		errStr := fmt.Sprintf("%s cluster version %%s is not supported.", context)

		major := version.Major
		minVersion, ok := minVersions[int(major)]
		if !ok {
			minVersion = minSourceVersion()
		}

		errStr = fmt.Sprintf(errStr, version)
		errStr = fmt.Sprintf("%s  The minimum required version is %s. We recommend the latest version.",
			errStr, minVersion)

		err = fmt.Errorf(errStr)
	} else if err != nil {
		err = fmt.Errorf("could not determine %s cluster version: %w", context, err)
	}
	return err
}
