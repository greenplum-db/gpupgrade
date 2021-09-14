//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package greenplum_test

import (
	"testing"

	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
)

var v5X = semver.MustParse("5.0.0")
var v6X = semver.MustParse("6.0.0")
var v7X = semver.MustParse("7.0.0")

func TestConnURI(t *testing.T) {
	testlog.SetupLogger()

	cases := []struct {
		name     string
		source   semver.Version
		target   semver.Version
		options  []greenplum.Option
		expected string
	}{
		{
			"default string",
			v5X,
			v6X,
			[]greenplum.Option{},
			"postgresql://localhost:0/template1?search_path=",
		},
		{
			"set port to a value",
			v5X,
			v6X,
			[]greenplum.Option{
				greenplum.Port(12345),
			},
			"postgresql://localhost:12345/template1?search_path=",
		},
		{
			"connect to source version less than 7X",
			v5X,
			v6X,
			[]greenplum.Option{
				greenplum.ToSource(),
				greenplum.UtilityMode(),
			},
			"postgresql://localhost:0/template1?search_path=&gp_session_role=utility",
		},
		{
			"connect to target version of 7X",
			v6X,
			v7X,
			[]greenplum.Option{
				greenplum.ToTarget(),
				greenplum.UtilityMode(),
			},
			"postgresql://localhost:0/template1?search_path=&gp_role=utility",
		},
		{
			"allow system table mods",
			v6X,
			v7X,
			[]greenplum.Option{
				greenplum.AllowSystemTableMods(),
			},
			"postgresql://localhost:0/template1?search_path=&allow_system_table_mods=true",
		},
		{
			"set all options to a 7X target",
			v6X,
			v7X,
			[]greenplum.Option{
				greenplum.ToTarget(),
				greenplum.Port(12345),
				greenplum.UtilityMode(),
				greenplum.AllowSystemTableMods(),
			},
			"postgresql://localhost:12345/template1?search_path=&gp_role=utility&allow_system_table_mods=true",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			conn := greenplum.Connection(c.source, c.target)

			actual := conn.URI(c.options...)
			if actual != c.expected {
				t.Errorf("got %q, want %q", actual, c.expected)
			}
		})
	}
}
