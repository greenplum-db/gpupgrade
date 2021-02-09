// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"testing"
)

func TestFormatViewOrRuleDdl(t *testing.T) {
	tests := []struct {
		name    string
		tokens  []string
		want    string
		wantErr bool
	}{
		{
			name:    "formats view with create view and select in two separate lines",
			tokens:  []string{"CREATE", "VIEW", "myview", "AS", "SELECT", "name", "FROM", "mytable", ";"},
			want:    "CREATE VIEW myview AS\nSELECT name FROM mytable ;",
			wantErr: false,
		},
		{
			name:    "formats rule with create view and body in single lines",
			tokens:  []string{"CREATE", "RULE", "myrule", "AS", "ON", "INSERT", "TO", "public.bar_ao", "DO", "INSTEAD", "DELETE", "FROM", "public.foo_ao;"},
			want:    "CREATE RULE myrule AS ON INSERT TO public.bar_ao DO INSTEAD DELETE FROM public.foo_ao;",
			wantErr: false,
		},
		{
			name:    "returns error if token list does not contain atleast 4 elements",
			tokens:  []string{"CREATE", "RULE", "myrule"},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatViewOrRuleDdl(tt.tokens)

			if err == nil && tt.wantErr {
				t.Errorf("expect an error")
			}

			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsViewOrRuleDdl(t *testing.T) {
	tests := []struct {
		name     string
		buf      []string
		line     string
		expected bool
	}{
		{
			name:     "line contains create view statement",
			buf:      []string{"-- Name: myview; Type: VIEW; Schema: public; Owner: gpadmin"},
			line:     "CREATE VIEW myview AS",
			expected: true,
		},
		{
			name:     "line contains create rule statement",
			buf:      []string{"-- Name: myrule; Type: RULE; Schema: public; Owner: gpadmin"},
			line:     "CREATE RULE myrule AS",
			expected: true,
		},
		{
			name:     "buffer does not contains view / rule identifier",
			buf:      []string{"-- Name: myrule; Type: TABLE; Schema: public; Owner: gpadmin"},
			line:     "CREATE TABLE mytable AS",
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if actual := IsViewOrRuleDdl(tt.buf, tt.line); actual != tt.expected {
				t.Errorf("got %t, want %t", actual, tt.expected)
			}
		})
	}
}
