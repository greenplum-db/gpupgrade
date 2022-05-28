// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestCheckEnvironmentOnSegments(t *testing.T) {
	testlog.SetupTestLogger()

	source := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "coordinator", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby", Port: 16432, Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Port: 25433, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg1", Port: 25434, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Port: 25435, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg2", Port: 25436, Role: greenplum.MirrorRole},
	})
	source.GPHome = "/usr/local/greenplum-db-source"

	intermediate := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "coordinator", DataDir: "/data/qddir/seg.HqtFHX54y0o.-1", Port: 50432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby.HqtFHX54y0o", Port: 50433, Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg.HqtFHX54y0o.1", Port: 50434, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg.HqtFHX54y0o.1", Port: 50435, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg.HqtFHX54y0o.2", Port: 50436, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg.HqtFHX54y0o.2", Port: 50437, Role: greenplum.MirrorRole},
	})
	intermediate.GPHome = "/usr/local/greenplum-db-target"

	t.Run("checks environment on segments", func(t *testing.T) {
		hub.SetPathCommand(exectest.NewCommand(hub.PathMain))
		defer hub.ResetPathCommand()

		hub.SetLdLibraryPathCommand(exectest.NewCommand(hub.LdLibraryPathMain))
		defer hub.ResetLdLibraryPathCommand()

		err := hub.CheckEnvironment(append(hub.AgentHosts(source), source.CoordinatorHostname()), source.GPHome, intermediate.GPHome)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("returns error when failing to check PATH on segments", func(t *testing.T) {
		hub.SetPathCommand(exectest.NewCommand(hub.Failure))
		defer hub.ResetPathCommand()

		hub.SetLdLibraryPathCommand(exectest.NewCommand(hub.LdLibraryPathMain))
		defer hub.ResetLdLibraryPathCommand()

		err := hub.CheckEnvironment(append(hub.AgentHosts(source), source.CoordinatorHostname()), source.GPHome, intermediate.GPHome)
		var expected utils.NextActionErr
		if !errors.As(err, &expected) {
			t.Fatalf("got type %T, want type %T", err, expected)
		}

		if !reflect.DeepEqual(err, expected) {
			t.Fatalf("got err %#v, want %#v", err, expected)
		}
	})

	t.Run("returns error when failing to check LD_LIBRARY_PATH on segments", func(t *testing.T) {
		hub.SetPathCommand(exectest.NewCommand(hub.PathMain))
		defer hub.ResetPathCommand()

		hub.SetLdLibraryPathCommand(exectest.NewCommand(hub.Failure))
		defer hub.ResetLdLibraryPathCommand()

		err := hub.CheckEnvironment(append(hub.AgentHosts(source), source.CoordinatorHostname()), source.GPHome, intermediate.GPHome)
		var expected utils.NextActionErr
		if !errors.As(err, &expected) {
			t.Fatalf("got type %T, want type %T", err, expected)
		}

		if !reflect.DeepEqual(err, expected) {
			t.Fatalf("got err %#v, want %#v", err, expected)
		}
	})
}

func TestCheckEnvironmentOnSegment(t *testing.T) {
	testlog.SetupTestLogger()

	sourceGphome := "/usr/local/greenplum-db-source"
	intermediateGphome := "/usr/local/greenplum-db-target"

	t.Run("success cases", func(t *testing.T) {
		cases := []struct {
			name          string
			path          string
			ldLibraryPath string
		}{
			{
				name: "succeeds when PATH does not contain source cluster GPHOME",
				path: "/usr/local/pxf-gp5/bin:/usr/local/greenplum-cc-4.11.1/bin:/usr/local/bin:/usr/bin:/usr/local/sbin:/usr/sbin:/usr/local/greenplum-cloud:/home/gpadmin/.local/bin:/home/gpadmin/bin",
			},
			{
				name:          "succeeds when LD_LIBRARY_PATH does not contain source cluster GPHOME",
				ldLibraryPath: "",
			},
		}

		host := "mdw"
		utils.System.Hostname = func() (string, error) {
			return host, nil
		}

		hub.SetPathCommand(exectest.NewCommand(hub.PathMain))
		defer hub.ResetPathCommand()

		hub.SetLdLibraryPathCommand(exectest.NewCommand(hub.LdLibraryPathMain))
		defer hub.ResetLdLibraryPathCommand()

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				resetPath := testutils.SetEnv(t, "PATH", c.path)
				defer resetPath()

				resetLDLibraryPath := testutils.SetEnv(t, "LD_LIBRARY_PATH", c.ldLibraryPath)
				defer resetLDLibraryPath()

				err := hub.CheckEnvironmentOnSegment(host, sourceGphome, intermediateGphome)
				if err != nil {
					t.Errorf("unexpected error %#v", err)
				}
			})
		}
	})

	t.Run("error cases", func(t *testing.T) {
		errorCases := []struct {
			name          string
			path          string
			ldLibraryPath string
			expected      string
		}{
			{
				name:     "errors when PATH contains source cluster GPHOME",
				path:     fmt.Sprintf("%[1]s/ext/R-3.3.3/bin:%[1]s/bin:%[1]s/ext/python/bin:/usr/local/pxf-gp5/bin:/usr/local/greenplum-cc-4.11.1/bin:/usr/local/bin:/usr/bin:/usr/local/sbin:/usr/sbin:/usr/local/greenplum-cloud:/home/gpadmin/.local/bin:/home/gpadmin/bin", sourceGphome),
				expected: "PATH contains GPHOME",
			},
			{
				name:     "errors when PATH contains target cluster GPHOME",
				path:     fmt.Sprintf("%[1]s/ext/R-3.3.3/bin:%[1]s/bin:%[1]s/ext/python/bin:/usr/local/pxf-gp5/bin:/usr/local/greenplum-cc-4.11.1/bin:/usr/local/bin:/usr/bin:/usr/local/sbin:/usr/sbin:/usr/local/greenplum-cloud:/home/gpadmin/.local/bin:/home/gpadmin/bin", intermediateGphome),
				expected: "PATH contains GPHOME",
			},
			{
				name:          "errors when LD_LIBRARY_PATH contains source cluster GPHOME",
				ldLibraryPath: fmt.Sprintf("%[1]s/ext/R-3.3.3/lib:%[1]s/ext/R-3.3.3/extlib:%[1]s/lib:%[1]s/ext/python/lib", sourceGphome),
				expected:      "LD_LIBRARY_PATH contains GPHOME",
			},
			{
				name:          "errors when LD_LIBRARY_PATH contains target cluster GPHOME",
				ldLibraryPath: fmt.Sprintf("%[1]s/ext/R-3.3.3/lib:%[1]s/ext/R-3.3.3/extlib:%[1]s/lib:%[1]s/ext/python/lib", intermediateGphome),
				expected:      "LD_LIBRARY_PATH contains GPHOME",
			},
		}

		host := "mdw"
		utils.System.Hostname = func() (string, error) {
			return host, nil
		}

		hub.SetPathCommand(exectest.NewCommand(hub.PathMain))
		defer hub.ResetPathCommand()

		hub.SetLdLibraryPathCommand(exectest.NewCommand(hub.LdLibraryPathMain))
		defer hub.ResetLdLibraryPathCommand()

		for _, c := range errorCases {
			t.Run(c.name, func(t *testing.T) {
				resetPath := testutils.SetEnv(t, "PATH", c.path)
				defer resetPath()

				resetLDLibraryPath := testutils.SetEnv(t, "LD_LIBRARY_PATH", c.ldLibraryPath)
				defer resetLDLibraryPath()

				err := hub.CheckEnvironmentOnSegment(host, sourceGphome, intermediateGphome)
				if !strings.Contains(err.Error(), c.expected) {
					t.Errorf("got %+v, want %+v", err, c.expected)
				}
			})
		}
	})
}
