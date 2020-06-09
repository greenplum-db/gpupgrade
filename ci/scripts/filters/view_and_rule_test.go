// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"reflect"
	"testing"
)

func TestBuildViewOrRuleDdl(t *testing.T) {
	type args struct {
		line      string
		allTokens []string
	}
	type result struct {
		line               string
		finishedFormatting bool
		resultTokens       []string
	}
	tests := []struct {
		name   string
		args   args
		result result
	}{
		{
			name: "returns completed ddl",
			args: args{
				line:      "SELECT name FROM mytable;",
				allTokens: []string{"CREATE", "VIEW", "myview", "AS"},
			},
			result: result{
				line:               "CREATE VIEW myview AS\nSELECT name FROM mytable;",
				resultTokens:       []string{"CREATE", "VIEW", "myview", "AS", "SELECT", "name", "FROM", "mytable;"},
				finishedFormatting: true,
			},
		},
		{
			name: "still processing view ddl",
			args: args{
				line:      "CREATE VIEW myview AS",
				allTokens: nil,
			},
			result: result{
				finishedFormatting: false,
				resultTokens:       []string{"CREATE", "VIEW", "myview", "AS"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			completeDdl, resultTokens, finishedFormatting := BuildViewOrRuleDdl(tt.args.line, tt.args.allTokens)
			if completeDdl != tt.result.line {
				t.Errorf("got %v, want %v", completeDdl, tt.result.line)
			}
			if finishedFormatting != tt.result.finishedFormatting {
				t.Errorf("got %t, want %t", finishedFormatting, tt.result.finishedFormatting)
			}
			if !reflect.DeepEqual(resultTokens, tt.result.resultTokens) {
				t.Errorf("got %q, want %q", resultTokens, tt.result.resultTokens)
			}
		})
	}
}

func TestFormatViewOrRuleDdl(t *testing.T) {
	tests := []struct {
		name      string
		allTokens []string
		want      string
	}{
		{
			name:      "formats view with create view and select in two separate lines",
			allTokens: []string{"CREATE", "VIEW", "myview", "AS", "SELECT", "name", "FROM", "mytable", ";"},
			want:      "CREATE VIEW myview AS\nSELECT name FROM mytable ;",
		},
		{
			name:      "formats rule with create view and body in single lines",
			allTokens: []string{"CREATE", "RULE", "myrule", "AS", "ON", "INSERT", "TO", "public.bar_ao", "DO", "INSTEAD", "DELETE", "FROM", "public.foo_ao;"},
			want:      "CREATE RULE myrule AS ON INSERT TO public.bar_ao DO INSTEAD DELETE FROM public.foo_ao;",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatViewOrRuleDdl(tt.allTokens); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsViewOrRuleDdl(t *testing.T) {
	type args struct {
		buf  []string
		line string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "buffer is empty",
			args: args{
				buf:  nil,
				line: "",
			},
			want: false,
		},
		{
			name: "buffer contains identifier for view comment and line contains create view statement",
			args: args{
				buf:  []string{"-- Name: myview; Type: VIEW; Schema: public; Owner: gpadmin"},
				line: "CREATE VIEW myview AS",
			},
			want: true,
		},
		{
			name: "buffer contains identifier for rule comment and line contains create rule statement",
			args: args{
				buf:  []string{"-- Name: bar_ao two; Type: RULE; Schema: public; Owner: gpadmin"},
				line: "CREATE RULE myrule AS",
			},
			want: true,
		},
		{
			name: "buffer does not contains view / rule identifier",
			args: args{
				buf:  []string{"-- Name: lineitem; Type: TABLE; Schema: public; Owner: gpadmin; Tablespace:"},
				line: "CREATE TABLE mytable AS",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsViewOrRuleDdl(tt.args.buf, tt.args.line); got != tt.want {
				t.Errorf("got %t, want %t", got, tt.want)
			}
		})
	}
}
