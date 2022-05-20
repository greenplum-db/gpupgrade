// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum_test

import (
	"testing"

	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
)

func TestConnection(t *testing.T) {
	testlog.SetupLogger()

	cases := []struct {
		name     string
		version  semver.Version
		options  []greenplum.Option
		expected string
	}{
		{
			"defaults to coordinator port",
			semver.MustParse("5.0.0"),
			[]greenplum.Option{},
			"postgresql://localhost:15432/template1?search_path=",
		},
		{
			"uses specified port value",
			semver.MustParse("5.0.0"),
			[]greenplum.Option{
				greenplum.Port(12345),
			},
			"postgresql://localhost:12345/template1?search_path=",
		},
		{
			"uses correct utility mode parameter when connecting to a 5X cluster",
			semver.MustParse("5.0.0"),
			[]greenplum.Option{
				greenplum.UtilityMode(),
			},
			"postgresql://localhost:15432/template1?search_path=&gp_session_role=utility",
		},
		{
			"uses correct utility mode parameter when connecting to a 6X cluster",
			semver.MustParse("6.0.0"),
			[]greenplum.Option{
				greenplum.UtilityMode(),
			},
			"postgresql://localhost:15432/template1?search_path=&gp_session_role=utility",
		},
		{
			"uses correct utility mode parameter when connecting to a 7X cluster",
			semver.MustParse("7.0.0"),
			[]greenplum.Option{
				greenplum.UtilityMode(),
			},
			"postgresql://localhost:15432/template1?search_path=&gp_role=utility",
		},
		{
			"allow system table mods",
			semver.MustParse("6.0.0"),
			[]greenplum.Option{
				greenplum.AllowSystemTableMods(),
			},
			"postgresql://localhost:15432/template1?search_path=&allow_system_table_mods=true",
		},
		{
			"can set multiple options",
			semver.MustParse("6.0.0"),
			[]greenplum.Option{
				greenplum.Port(1234),
				greenplum.UtilityMode(),
				greenplum.AllowSystemTableMods(),
			},
			"postgresql://localhost:1234/template1?search_path=&gp_session_role=utility&allow_system_table_mods=true",
		},
	}

	source := MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
	})

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			source.Version = c.version

			actual := source.Connection(c.options...)
			if actual != c.expected {
				t.Errorf("got %q, want %q", actual, c.expected)
			}
		})
	}
}
