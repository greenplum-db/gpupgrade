// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"testing"
)

func TestFormatTriggerDdl(t *testing.T) {
	tests := []struct {
		name    string
		tokens  []string
		want    string
		wantErr bool
	}{
		{
			name: "formats create trigger statement and body in to one line",
			tokens: []string{"CREATE", "TRIGGER", "after_trigger", "AFTER", "INSERT", "OR", "DELETE", "ON",
				"public.foo", "FOR", "EACH", "ROW", "EXECUTE", "PROCEDURE", "public.bfv_dml_error_func();"},
			want:    "CREATE TRIGGER after_trigger\n    AFTER INSERT OR DELETE ON public.foo\n    FOR EACH ROW\n    EXECUTE PROCEDURE public.bfv_dml_error_func();",
			wantErr: false,
		},
		{
			name:    "formats create trigger statement and body in to one line",
			tokens:  []string{},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatTriggerDdl(tt.tokens)

			if err == nil && tt.wantErr {
				t.Errorf("expect an error")
			}

			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsTriggerDdl(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		result bool
	}{
		{
			name:   "line contains create trigger statement",
			line:   "CREATE TRIGGER mytrigger AS",
			result: true,
		},
		{
			name:   "line does not create trigger statement",
			line:   "CREATE TABLE mytable AS",
			result: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTriggerDdl(tt.line); got != tt.result {
				t.Errorf("got %t, want %t", got, tt.result)
			}
		})
	}
}
