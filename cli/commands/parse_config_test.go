// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/cli/commands"
)

func TestConfig(t *testing.T) {
	cases := []struct {
		description      string
		config           string
		parameter        string
		expected         string
		shouldBeExcluded bool
	}{
		{
			description: "parses parameter",
			config:      `name = value`,
			parameter:   "name",
			expected:    `value`,
		},
		{
			description: "parses parameters without whitespace",
			config:      `name=value`,
			parameter:   "name",
			expected:    `value`,
		},
		{
			description: "replaces _ with - in parameter names",
			config:      `name_with_dash = value`,
			parameter:   "name-with-dash",
			expected:    `value`,
		},
		{
			description: "allows values with spaces when there is an equal sign",
			config:      `name = value with spaces`,
			parameter:   "name",
			expected:    `value with spaces`,
		},
		{
			description: "parses values containing spaces with extra whitespace",
			config:      `name =     value with spaces    `,
			parameter:   "name",
			expected:    `value with spaces`,
		},
		{
			description: "parses values containing single and double quotes",
			config:      `name = value with "double" and 'single' quotes`,
			parameter:   "name",
			expected:    `value with "double" and 'single' quotes`,
		},
		{
			description:      "ignores empty lines",
			config:           "   \n  \t",
			parameter:        "name",
			expected:         ``,
			shouldBeExcluded: true,
		},
		{
			description:      "ignores leading comments",
			config:           `# comment`,
			parameter:        "name",
			expected:         ``,
			shouldBeExcluded: true,
		},
		{
			description: "ignores inline comments",
			config:      `name = value # comment`,
			parameter:   "name",
			expected:    `value`,
		},
		{
			description: "ignores inline comments containing equal signs",
			config:      `name = value # comment with = sign`,
			parameter:   "name",
			expected:    `value`,
		},
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			flags, err := commands.ParseConfig(strings.NewReader(c.config))
			if err != nil {
				t.Errorf("ParseConfig returned error: %+v", err)
			}

			value, ok := flags[c.parameter]
			if c.shouldBeExcluded && ok {
				t.Errorf("expected paramter %q to not be present", c.parameter)
			}

			if value != c.expected {
				t.Errorf("flags[%q] = %q, want %q", c.parameter, value, c.expected)
			}
		})
	}

	errorCases := []struct {
		description string
		config      string
	}{
		{
			description: "parameter missing equal sign",
			config:      `name value`,
		},
		{
			description: "parameter value with spaces missing equal sign",
			config:      `name value with spaces`,
		},
		{
			description: "parameter is specified multiple times",
			config:      "config_name = value\nconfig_name = value2",
		},
		{
			description: "parameter value is empty with equal sign",
			config:      "name = ",
		},
		{
			description: "parameter value is empty without equal sign",
			config:      "name ",
		},
		{
			description: "parameter value is empty with an inline comment",
			config:      "name # comment",
		},
		{
			description: "parameter value is empty with equal sign and inline comment containing an equal sign",
			config:      "name = # comment name = ",
		},
		{
			description: "parameter value is empty without equal sign and inline comment containing an equal sign",
			config:      "name # comment name = ",
		},
	}

	for _, c := range errorCases {
		t.Run(fmt.Sprintf("errors when %s", c.description), func(t *testing.T) {
			_, err := commands.ParseConfig(strings.NewReader(c.config))
			if err == nil {
				t.Errorf("expected error %#v got nil", err)
			}
		})
	}
}
