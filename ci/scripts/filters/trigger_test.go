// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"testing"
)

func TestFormatTriggerDdl(t *testing.T) {
	tests := []struct {
		name        string
		tokens      []string
		expected    string
		expectedErr bool
	}{
		{
			name: "formats create trigger statement and body in to one line",
			tokens: []string{"CREATE", "TRIGGER", "after_trigger", "AFTER", "INSERT", "OR", "DELETE", "ON",
				"public.foo", "FOR", "EACH", "ROW", "EXECUTE", "PROCEDURE", "public.bfv_dml_error_func();"},
			expected:    "CREATE TRIGGER after_trigger\n    AFTER INSERT OR DELETE ON public.foo\n    FOR EACH ROW\n    EXECUTE PROCEDURE public.bfv_dml_error_func();",
			expectedErr: false,
		},
		{
			name:        "formats create trigger statement and body in to one line",
			tokens:      []string{},
			expected:    "",
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := FormatTriggerDdl(tt.tokens)

			if err == nil && tt.expectedErr {
				t.Errorf("expect an error")
			}

			if actual != tt.expected {
				t.Errorf("got %q, want %q", actual, tt.expected)
			}
		})
	}
}

func TestIsTriggerDdl(t *testing.T) {
	tests := []struct {
		name     string
		buf      []string
		line     string
		expected bool
	}{
		{
			name:     "line contains create trigger statement",
			buf:      []string{"-- Name: mytrigger; Type: TRIGGER; Schema: public; Owner: gpadmin"},
			line:     "CREATE TRIGGER mytrigger AS",
			expected: true,
		},
		{
			name:     "line does not create trigger statement",
			buf:      []string{"-- Name: mytable; Type: TABLE; Schema: public; Owner: gpadmin"},
			line:     "CREATE TABLE mytable AS",
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if actual := IsTriggerDdl(tt.buf, tt.line); actual != tt.expected {
				t.Errorf("got %t, want %t", actual, tt.expected)
			}
		})
	}
}
