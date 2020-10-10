// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
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
		name   string
		line   string
		result bool
	}{
		{
			name:   "line contains create view statement",
			line:   "CREATE VIEW myview AS",
			result: true,
		},
		{
			name:   "line contains create rule statement",
			line:   "CREATE RULE myrule AS",
			result: true,
		},
		{
			name:   "buffer does not contains view / rule identifier",
			line:   "CREATE TABLE mytable AS",
			result: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsViewOrRuleDdl(tt.line); got != tt.result {
				t.Errorf("got %t, want %t", got, tt.result)
			}
		})
	}
}
