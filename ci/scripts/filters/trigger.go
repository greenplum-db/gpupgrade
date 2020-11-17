// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"errors"
	"regexp"
	"strings"
)

var (
	triggerCommentRegex *regexp.Regexp
	triggerCreateRegex  *regexp.Regexp
)

func init() {
	triggerCommentRegex = regexp.MustCompile(`; Type: TRIGGER;`)
	triggerCreateRegex = regexp.MustCompile(`CREATE TRIGGER `)
}

func IsTriggerDdl(buf []string, line string) bool {
	return len(buf) > 0 && triggerCommentRegex.MatchString(strings.Join(buf, " ")) && triggerCreateRegex.MatchString(line)
}

func FormatTriggerDdl(tokens []string) (string, error) {
	if len(tokens) == 0 {
		return "", errors.New("tokens cannot be of zero length")
	}

	var line string
	for _, token := range tokens {
		if line == "" {
			// processing the first element
			line = token
			continue
		}

		// by default add single space between tokens, but if a token is identified which marks a new line
		// use a new line and 4 character space indentation to match the format of old dump
		indentation := " "
		for _, identifier := range []string{"AFTER", "BEFORE", "FOR", "EXECUTE"} {
			if token == identifier {
				indentation = "\n    "
				break
			}
		}

		line = line + indentation + token
	}

	return line, nil
}
