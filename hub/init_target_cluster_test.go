// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/blang/semver/v4"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"
)

func gpinitsystem() {}

func gpinitsystem_Exits1() {
	os.Stdout.WriteString("[WARN]:-Master open file limit is 256 should be >= 65535")
	os.Exit(1)
}

func pg_controldata() {
	os.Stdout.WriteString(`
pg_control version number:            9420600
Catalog version number:               301908232
Database system identifier:           6849079892457217099
Database cluster state:               in production
pg_control last modified:             Mon Jul 13 14:36:28 2020
Latest checkpoint location:           0/180001D0
Prior checkpoint location:            0/18000150
Latest checkpoint's REDO location:    0/180001D0
`)
}

func init() {
	exectest.RegisterMains(
		gpinitsystem,
		gpinitsystem_Exits1,
		pg_controldata,
	)
}

func TestCreateInitialInitsystemConfig(t *testing.T) {
	t.Run("successfully get initial gpinitsystem config array", func(t *testing.T) {
		utils.System.Hostname = func() (string, error) {
			return "mdw", nil
		}

		actualConfig, err := CreateInitialInitsystemConfig("/data/qddir/seg.AAAAAAAAAAA.-1")
		if err != nil {
			t.Fatalf("got %#v, want nil", err)
		}

		expectedConfig := []string{
			`ARRAY_NAME="gp_upgrade cluster"`,
			"SEG_PREFIX=seg.AAAAAAAAAAA.",
			"TRUSTED_SHELL=ssh",
		}
		if !reflect.DeepEqual(actualConfig, expectedConfig) {
			t.Errorf("got %v, want %v", actualConfig, expectedConfig)
		}
	})
}

func TestGetCheckpointSegmentsAndEncoding(t *testing.T) {
	t.Run("successfully get the GUC values", func(t *testing.T) {
		dbConn, sqlMock := testhelper.CreateAndConnectMockDB(1)

		checkpointRow := sqlmock.NewRows([]string{"string"}).AddRow(driver.Value("8"))
		encodingRow := sqlmock.NewRows([]string{"string"}).AddRow(driver.Value("UNICODE"))
		sqlMock.ExpectQuery("SELECT .*checkpoint.*").WillReturnRows(checkpointRow)
		sqlMock.ExpectQuery("SELECT .*server.*").WillReturnRows(encodingRow)

		actualConfig, err := GetCheckpointSegmentsAndEncoding([]string{}, dbConn)
		if err != nil {
			t.Fatalf("got %#v, want nil", err)
		}

		expectedConfig := []string{"CHECK_POINT_SEGMENTS=8", "ENCODING=UNICODE"}
		if !reflect.DeepEqual(actualConfig, expectedConfig) {
			t.Errorf("got %v, want %v", actualConfig, expectedConfig)
		}
	})
}

func TestWriteSegmentArray(t *testing.T) {
	test := func(t *testing.T, initializeConfig InitializeConfig, expected []string) {
		t.Helper()

		actual, err := WriteSegmentArray([]string{}, initializeConfig)
		if err != nil {
			t.Errorf("got %#v", err)
		}

		if !reflect.DeepEqual(actual, expected) {
			// Help developers see differences between the lines.
			pretty := func(lines []string) string {
				b := new(strings.Builder)

				fmt.Fprintln(b, "[")
				for _, l := range lines {
					fmt.Fprintf(b, "  %q\n", l)
				}
				fmt.Fprint(b, "]")

				return b.String()
			}
			t.Errorf("got %v, want %v", pretty(actual), pretty(expected))
		}
	}

	t.Run("renders the config file as expected", func(t *testing.T) {
		config := InitializeConfig{
			Master: greenplum.SegConfig{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir_upgrade/seg-1", Role: "p", Port: 15433},
			Primaries: []greenplum.SegConfig{
				{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1_upgrade/seg1", Role: "p", Port: 15434},
				{ContentID: 1, DbID: 3, Hostname: "sdw2", DataDir: "/data/dbfast2_upgrade/seg2", Role: "p", Port: 15434},
			},
		}

		test(t, config, []string{
			"QD_PRIMARY_ARRAY=mdw~15433~/data/qddir_upgrade/seg-1~1~-1",
			"declare -a PRIMARY_ARRAY=(",
			"\tsdw1~15434~/data/dbfast1_upgrade/seg1~2~0",
			"\tsdw2~15434~/data/dbfast2_upgrade/seg2~3~1",
			")",
		})
	})

	t.Run("errors when source cluster contains no master segment", func(t *testing.T) {
		_, err := WriteSegmentArray([]string{}, InitializeConfig{})

		if err == nil {
			t.Errorf("expected error got nil")
		}
	})
}

func TestRunInitsystemForTargetCluster(t *testing.T) {
	gpHome6 := "/usr/local/gpdb6"
	version6 := semver.MustParse("6.0.0")

	gpHome7 := "/usr/local/gpdb7"
	version7 := semver.MustParse("7.0.0")

	gpinitsystemConfigPath := "/dir/.gpupgrade/gpinitsystem_config"

	execCommand = nil
	defer func() {
		execCommand = nil
	}()

	t.Run("does not use --ignore-warnings when upgrading to GPDB7 or higher", func(t *testing.T) {
		execCommand = exectest.NewCommandWithVerifier(gpinitsystem,
			func(path string, args ...string) {
				if path != "bash" {
					t.Errorf("executed %q, want bash", path)
				}

				expected := []string{"-c", "source /usr/local/gpdb7/greenplum_path.sh && " +
					"/usr/local/gpdb7/bin/gpinitsystem -a -I /dir/.gpupgrade/gpinitsystem_config"}
				if !reflect.DeepEqual(args, expected) {
					t.Errorf("args %q, want %q", args, expected)
				}
			})

		err := RunInitsystemForTargetCluster(step.DevNullStream, gpHome7, gpinitsystemConfigPath, version7)
		if err != nil {
			t.Error("gpinitsystem failed")
		}
	})

	t.Run("only uses --ignore-warnings when upgrading to GPDB6", func(t *testing.T) {
		execCommand = exectest.NewCommandWithVerifier(gpinitsystem,
			func(path string, args ...string) {
				if path != "bash" {
					t.Errorf("executed %q, want bash", path)
				}

				expected := []string{"-c", "source /usr/local/gpdb6/greenplum_path.sh && " +
					"/usr/local/gpdb6/bin/gpinitsystem -a -I /dir/.gpupgrade/gpinitsystem_config --ignore-warnings"}
				if !reflect.DeepEqual(args, expected) {
					t.Errorf("args %q, want %q", args, expected)
				}
			})

		err := RunInitsystemForTargetCluster(step.DevNullStream, gpHome6, gpinitsystemConfigPath, version6)
		if err != nil {
			t.Error("gpinitsystem failed")
		}
	})

	t.Run("returns an error when gpinitsystem fails with --ignore-warnings when upgrading to GPDB6", func(t *testing.T) {
		execCommand = exectest.NewCommand(gpinitsystem_Exits1)

		err := RunInitsystemForTargetCluster(step.DevNullStream, gpHome6, gpinitsystemConfigPath, version6)

		var actual *exec.ExitError
		if !xerrors.As(err, &actual) {
			t.Fatalf("got %#v, want ExitError", err)
		}

		if actual.ExitCode() != 1 {
			t.Errorf("got %d, want 1 ", actual.ExitCode())
		}
	})

	t.Run("returns an error when gpinitsystem errors when upgrading to GPDB7 or higher", func(t *testing.T) {
		execCommand = exectest.NewCommand(gpinitsystem_Exits1)

		err := RunInitsystemForTargetCluster(step.DevNullStream, gpHome7, gpinitsystemConfigPath, version7)

		var actual *exec.ExitError
		if !xerrors.As(err, &actual) {
			t.Fatalf("got %#v, want ExitError", err)
		}

		if actual.ExitCode() != 1 {
			t.Errorf("got %d, want 1", actual.ExitCode())
		}
	})
}

func TestGetMasterSegPrefix(t *testing.T) {
	t.Run("returns a valid seg prefix given", func(t *testing.T) {
		cases := []struct {
			desc          string
			MasterDataDir string
		}{
			{"an absolute path", "/data/master/gpseg-1"},
			{"a relative path", "../master/gpseg-1"},
			{"a implicitly relative path", "gpseg-1"},
		}

		for _, c := range cases {
			actual, err := GetMasterSegPrefix(c.MasterDataDir)
			if err != nil {
				t.Fatalf("got %#v, want nil", err)
			}

			expected := "gpseg"
			if actual != expected {
				t.Errorf("got %q, want %q", actual, expected)
			}
		}
	})

	t.Run("returns errors when given", func(t *testing.T) {
		cases := []struct {
			desc          string
			MasterDataDir string
		}{
			{"the empty string", ""},
			{"a path without a content identifier", "/opt/myseg"},
			{"a path with a segment content identifier", "/opt/myseg2"},
			{"a path that is only a content identifier", "-1"},
			{"a path that ends in only a content identifier", "///-1"},
		}

		for _, c := range cases {
			_, err := GetMasterSegPrefix(c.MasterDataDir)
			if err == nil {
				t.Fatalf("got nil, want err")
			}
		}
	})
}

func TestGetCatalogVersion(t *testing.T) {
	testhelper.SetupTestLogger()

	gphome := "/usr/local/target"
	datadir := "/data/qddir_upgrade/seg-1"

	t.Run("returns catalog version", func(t *testing.T) {
		SetExecCommand(exectest.NewCommand(pg_controldata))
		defer ResetExecCommand()

		version, err := GetCatalogVersion(step.DevNullStream, gphome, datadir)
		if err != nil {
			t.Errorf("GetCatalogVersion returned error %+v", err)
		}

		expected := "301908232"
		if version != expected {
			t.Errorf("got %s want %s", version, expected)
		}
	})

	t.Run("errors when pg_controldata fails", func(t *testing.T) {
		SetExecCommand(exectest.NewCommand(Failure))
		defer ResetExecCommand()

		version, err := GetCatalogVersion(step.DevNullStream, gphome, datadir)
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("got error %#v want %T", err, exitErr)
		}

		if exitErr.ExitCode() != 1 {
			t.Errorf("got exit code %d want 1", exitErr.ExitCode())
		}

		if version != "" {
			t.Errorf("got version %s want empty string", version)
		}
	})

	t.Run("errors when catalog version is not found", func(t *testing.T) {
		SetExecCommand(exectest.NewCommand(Success))
		defer ResetExecCommand()

		version, err := GetCatalogVersion(step.DevNullStream, gphome, datadir)
		if !errors.Is(err, ErrUnknownCatalogVersion) {
			t.Errorf("got error %#v want %#v", err, ErrUnknownCatalogVersion)
		}

		if version != "" {
			t.Errorf("got version %s want empty string", version)
		}
	})
}
