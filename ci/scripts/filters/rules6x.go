// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"regexp"
)

func Init6x() {
	// linePatterns remove exactly what is matched, on a line-by-line basis.
	linePatterns := []string{
		`ALTER DATABASE .+ SET gp_use_legacy_hashops TO 'on';`,
		// TODO: There may be false positives because of the below
		// pattern, and we might have to do a look ahead to really identify
		// if it can be deleted.
		`START WITH \d`,
	}

	// blockPatterns remove lines that match, AND any comments or whitespace
	// immediately preceding them.
	blockPatterns := []string{
		"CREATE EXTENSION IF NOT EXISTS plpgsql WITH SCHEMA pg_catalog;",
		"COMMENT ON EXTENSION plpgsql IS",
		"COMMENT ON DATABASE postgres IS",
	}

	ReplacementFuncs = []ReplacementFunc{
		FormatWithClause,
		ReplacePrecision,
		Replacements,
	}

	// patten matching functions and corresponding formatting functions
	Formatters = []formatter{
		{shouldFormat: IsViewOrRuleDdl, format: FormatViewOrRuleDdl},
		{shouldFormat: IsTriggerDdl, format: FormatTriggerDdl},
	}

	for _, pattern := range linePatterns {
		LineRegexes = append(LineRegexes, regexp.MustCompile(pattern))
	}
	for _, pattern := range blockPatterns {
		BlockRegexes = append(BlockRegexes, regexp.MustCompile(pattern))
	}
}
