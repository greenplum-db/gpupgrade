// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package filters

import "testing"

func TestReplacePrecision(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{
			name: "returns the string as is if no replacement is performed",
			line: "adfasdfasdf	127.90.90.1	12.",
			want: "adfasdfasdf	127.90.90.1	12.",
		},
		{
			name: "replaces precision values of floating point number in tab delimited data",
			line: ".12	adfasdfasdf	127.90.90.1	12.12	.13",
			want: ".XX	adfasdfasdf	127.90.90.1	12.XX	.XX",
		},
		{
			name: "replaces comma separated values in parantheses brackets",
			line: "(0.90000000000000002, 6.0999999999999996)",
			want: "(0.XX, 6.XX)",
		},
		{
			name: "replaces precision values in ddl clause",
			line: "START (0::double precision) END (.89999999999999991::double precision) EVERY (1.2::double precision) WITH (tablename='multivarblock_parttab_1_prt_p1_2_prt_2', checksum=true, appendonly=true, orientation=column ) ",
			want: "START (0::double precision) END (.XX::double precision) EVERY (1.XX::double precision) WITH (tablename='multivarblock_parttab_1_prt_p1_2_prt_2', checksum=true, appendonly=true, orientation=column ) ",
		},
		{
			name: "replaces consecutive pairs of values in parentheses in square brackets",
			line: "State Hwy 92                  Ramp\t[(-122.1204,37.267000000000003),(-1.123,3.271000000000001)]",
			want: "State Hwy 92                  Ramp\t[(-122.XX,37.XX),(-1.XX,3.XX)]",
		},
		{
			name: "replaces precision values in curly braces with single quotes",
			line: "    large_content double precision[] DEFAULT '{1234567890.1199999,1234567890.11999993}'::double precision[] ",
			want: "    large_content double precision[] DEFAULT '{1234567890.XX,1234567890.XX}'::double precision[] ",
		},
		{
			name: "replaces precision values in curly braces",
			line: "[0:1]={1.1000000000000001,2.2000000000000002,3}",
			want: "[0:1]={1.XX,2.XX,3}",
		},
		{
			name: "replaces consecutive pairs of values in parentheses brackets",
			line: "0\tOakland\t((-122,37.899999999999999),(-121.7,37.899999999999999),(-121.7,37.399999999999999),(-122,37.399999999999999))",
			want: "0\tOakland\t((-122,37.XX),(-121.XX,37.XX),(-121.XX,37.XX),(-122,37.XX))",
		},
		{
			name: "excludes replacing precision values in ddl clause containing values function",
			line: "VALUES(0.06, 0.01, 0.02, 0.07, 0.08) WITH (tablename='aopart_lineitem_1_prt_p2_2_prt_4_3_prt_5_4_prt_2', orientation=row, appendonly=true, checksum=false, compresstype=zlib, compresslevel=1 )",
			want: "VALUES(0.06, 0.01, 0.02, 0.07, 0.08) WITH (tablename='aopart_lineitem_1_prt_p2_2_prt_4_3_prt_5_4_prt_2', orientation=row, appendonly=true, checksum=false, compresstype=zlib, compresslevel=1 )",
		},
		{
			name: "excludes replacing precision values if the line contains perform pg_sleep",
			line: "perform pg_sleep(0.05) [0:1]={1.23,2.45,3}",
			want: "perform pg_sleep(0.05) [0:1]={1.23,2.45,3}",
		},
		{
			name: "excludes replacing precision values if the line contains time.sleep",
			line: "time.sleep(0.05) [0:1]={1.23,2.45,3}",
			want: "time.sleep(0.05) [0:1]={1.23,2.45,3}",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ReplacePrecision(c.line)
			if got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}
