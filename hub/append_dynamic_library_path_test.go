//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
)

func TestAppendDynamicLibraryPath(t *testing.T) {
	testlog.SetupLogger()

	intermediate := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "master", DataDir: "/data/qddir/seg.HqtFHX54y0o.-1", Port: 50432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby.HqtFHX54y0o", Port: 50433, Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg.HqtFHX54y0o.1", Port: 50434, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg.HqtFHX54y0o.1", Port: 50435, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg.HqtFHX54y0o.2", Port: 50436, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg.HqtFHX54y0o.2", Port: 50437, Role: greenplum.MirrorRole},
	})

	t.Run("returns error when gpconfig fails", func(t *testing.T) {
		greenplum.SetGreenplumCommand(exectest.NewCommand(hub.Failure))
		defer greenplum.ResetGreenplumCommand()

		err := hub.AppendDynamicLibraryPath(intermediate, "")
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("got error %#v want %T", err, exitErr)
		}

		if exitErr.ExitCode() != 1 {
			t.Errorf("got exit code %d want 1", exitErr.ExitCode())
		}
	})

	t.Run("returns error when gpconfig returns no value for dynamic_library_path", func(t *testing.T) {
		greenplum.SetGreenplumCommand(exectest.NewCommand(hub.Success))
		defer greenplum.ResetGreenplumCommand()

		err := hub.AppendDynamicLibraryPath(intermediate, "")
		expected := "issing value for dynamic_library_path"
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("got %+v, want %+v", err, expected)
		}
	})
}
