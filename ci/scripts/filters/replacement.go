// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package filters

import "regexp"

type Replacer struct {
	Regex       *regexp.Regexp
	Replacement string
}

func (t *Replacer) replace(line string) string {
	return t.Regex.ReplaceAllString(line, t.Replacement)
}

func InitReplacementRegex(patterns map[string]string) []*Replacer {
	var replacer []*Replacer
	for regex, replacement := range patterns {
		replacer = append(replacer, &Replacer{
			Regex:       regexp.MustCompile(regex),
			Replacement: replacement,
		})
	}

	return replacer
}

func Replacements(line string) string {
	patterns := map[string]string{
		`(DEFAULT.*[^B])('\d+')::"bit"`: `${1}B${2}::"bit"`,
	}

	replacer := InitReplacementRegex(patterns)
	for _, r := range replacer {
		line = r.replace(line)
	}

	return line
}
