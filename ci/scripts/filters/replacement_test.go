// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"testing"
)

func TestReplacements(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     `append B to the pattern 'n'::"bit"`,
			line:     `a39 bit(1) DEFAULT '0'::"bit" ENCODING`,
			expected: `a39 bit(1) DEFAULT B'0'::"bit" ENCODING`,
		},
		{
			name:     `append B to the pattern ('n'::"bit")`,
			line:     `a40 bit varying(5) DEFAULT ('1'::"bit")::bit varying`,
			expected: `a40 bit varying(5) DEFAULT (B'1'::"bit")::bit varying`,
		},
		{
			name:     `does not append B to the pattern B'n'::"bit"`,
			line:     `a39 bit(1) DEFAULT B'0'::"bit" ENCODING`,
			expected: `a39 bit(1) DEFAULT B'0'::"bit" ENCODING`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := Replacements(tt.line)
			if actual != tt.expected {
				t.Errorf("got %v, expected %v", actual, tt.expected)
			}
		})
	}
}
