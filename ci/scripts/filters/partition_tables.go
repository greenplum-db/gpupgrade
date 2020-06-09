// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"regexp"
	"strings"
)

var withClauseRegex = regexp.MustCompile(`(.*WITH\s\(tablename[^,]*,)(.*)`)

func FormatWithClause(line string) string {
	result := withClauseRegex.FindAllStringSubmatch(line, -1)
	if result == nil {
		return line
	}
	groups := result[0]
	// replace all occurrences of single quotes
	stringWithoutSingleQuotes := strings.ReplaceAll(groups[2], "'", "")

	return groups[1] + stringWithoutSingleQuotes
}
