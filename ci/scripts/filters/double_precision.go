// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"regexp"
	"strings"
)

func ReplacePrecision(line string) string {
	excludeRegex := regexp.MustCompile(`VALUES.* WITH \(tablename|perform pg_sleep|time.sleep|COALESCE`)
	if excludeRegex.MatchString(line) {
		return line
	}

	floatRegex := regexp.MustCompile(`((?:^|[^\d\.]))(\d*)\.(\d+)((?:[^\d.]|$))`)
	match := floatRegex.FindStringSubmatch(line)
	for match != nil {
		match[3] = ".XX"
		line = strings.Replace(line, match[0], strings.Join(match[1:], ""), 1)
		match = floatRegex.FindStringSubmatch(line)
	}

	return line
}
