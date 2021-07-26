// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"testing"
)

func TestReplacements5X(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "does not replace the line if there is no default casting needed",
			line:     "CREATE FUNCTION public.st_addband(pixeltype text, nodataval double precision DEFAULT NULL::double precision) RETURNS public.raster",
			expected: "CREATE FUNCTION public.st_addband(pixeltype text, nodataval double precision DEFAULT NULL::double precision) RETURNS public.raster",
		},
		{
			name:     "replaces default casting parenthesis",
			line:     "CREATE FUNCTION public.st_addband(rast public.raster, pixeltype text, initialvalue double precision DEFAULT 0::numeric, nodataval double precision DEFAULT NULL::double precision) RETURNS public.raster",
			expected: "CREATE FUNCTION public.st_addband(rast public.raster, pixeltype text, initialvalue double precision DEFAULT (0)::numeric, nodataval double precision DEFAULT NULL::double precision) RETURNS public.raster",
		},
		{
			name:     "replaces default casting with decimal point parenthesis",
			line:     "CREATE FUNCTION public.st_addband(rast public.raster, pixeltype text, initialvalue double precision DEFAULT 0.1::numeric, nodataval double precision DEFAULT NULL::double precision) RETURNS public.raster",
			expected: "CREATE FUNCTION public.st_addband(rast public.raster, pixeltype text, initialvalue double precision DEFAULT (0.1)::numeric, nodataval double precision DEFAULT NULL::double precision) RETURNS public.raster",
		},
		{
			name:     "excludes replacing precision values if the line already contains default casting parenthesis",
			line:     "CREATE FUNCTION public.st_addband(rast public.raster, pixeltype text, initialvalue double precision DEFAULT (0)::numeric, nodataval double precision DEFAULT NULL::double precision) RETURNS public.raster",
			expected: "CREATE FUNCTION public.st_addband(rast public.raster, pixeltype text, initialvalue double precision DEFAULT (0)::numeric, nodataval double precision DEFAULT NULL::double precision) RETURNS public.raster",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Replacements5X(c.line)
			if got != c.expected {
				t.Errorf("got %v want %v", got, c.expected)
				t.Logf("actual:   %s", got)
				t.Logf("expected: %s", c.expected)
			}
		})
	}
}

func TestReplacements6X(t *testing.T) {
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
			actual := Replacements6X(tt.line)
			if actual != tt.expected {
				t.Errorf("got %v, expected %v", actual, tt.expected)
			}
		})
	}
}
