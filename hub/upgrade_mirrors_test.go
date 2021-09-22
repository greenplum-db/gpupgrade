// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"errors"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestWriteAddMirrorsConfig(t *testing.T) {
	t.Run("writes gpaddmirrors_config", func(t *testing.T) {
		stateDir := testutils.GetTempDir(t, "")
		defer os.RemoveAll(stateDir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
		defer resetEnv()

		intermediate := MustCreateCluster(t, greenplum.SegConfigs{
			{DbID: 1, ContentID: -1, Hostname: "master", DataDir: "/data/qddir/seg.HqtFHX54y0o.-1", Port: 50432, Role: greenplum.PrimaryRole},
			{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby.HqtFHX54y0o", Port: 50433, Role: greenplum.MirrorRole},
			{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg.HqtFHX54y0o.1", Port: 50434, Role: greenplum.PrimaryRole},
			{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg.HqtFHX54y0o.1", Port: 50435, Role: greenplum.MirrorRole},
			{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg.HqtFHX54y0o.2", Port: 50436, Role: greenplum.PrimaryRole},
			{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg.HqtFHX54y0o.2", Port: 50437, Role: greenplum.MirrorRole},
		})

		err := writeAddMirrorsConfig(intermediate)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		// iterating maps is not deterministic so sort and rejoin before asserting
		lines := strings.Split(testutils.MustReadFile(t, utils.GetAddMirrorsConfig()), "\n")
		sort.Strings(lines)
		actual := strings.Join(lines, "\n")

		expected := `
0|sdw2|50435|/data/dbfast_mirror1/seg.HqtFHX54y0o.1
1|sdw1|50437|/data/dbfast_mirror2/seg.HqtFHX54y0o.2`
		if actual != expected {
			t.Errorf("got %q want %q", actual, expected)
		}
	})
}

type greenplumStub struct {
	run func(utilityName string, arguments ...string) error
}

func (g *greenplumStub) Run(utilityName string, arguments ...string) error {
	return g.run(utilityName, arguments...)
}

func TestRunAddMirrors(t *testing.T) {
	t.Run("runs gpaddmirrors with the created config file", func(t *testing.T) {
		stateDir := testutils.GetTempDir(t, "")
		defer os.RemoveAll(stateDir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
		defer resetEnv()

		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		defer testutils.FinishMock(mock, t)
		defer db.Close()

		stub := &greenplumStub{
			run: func(utility string, args ...string) error {
				if utility != "gpaddmirrors" {
					t.Errorf("got utility %q want gpaddmirrors", utility)
				}

				expected := []string{
					"-a",
					"-i",
					utils.GetAddMirrorsConfig(),
					"--hba-hostnames",
				}

				if !reflect.DeepEqual(args, expected) {
					t.Errorf("got args %q want %q", args, expected)
				}

				return nil
			},
		}

		err = runGpAddMirrors(stub, true)
		if err != nil {
			t.Errorf("returned error %+v", err)
		}
	})

	t.Run("bubbles up errors from the utility", func(t *testing.T) {
		stateDir := testutils.GetTempDir(t, "")
		defer os.RemoveAll(stateDir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
		defer resetEnv()

		stub := new(greenplumStub)

		expected := errors.New("ahhhh")
		stub.run = func(_ string, _ ...string) error {
			return expected
		}

		err := runGpAddMirrors(stub, true)
		if !errors.Is(err, expected) {
			t.Errorf("returned error %#v, want %#v", err, expected)
		}
	})
}
