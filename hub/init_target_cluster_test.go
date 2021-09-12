// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"bufio"
	"bytes"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/blang/semver/v4"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
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

		actualConfig, err := CreateInitialInitsystemConfig("/data/qddir/seg.AAAAAAAAAAA.-1", true)
		if err != nil {
			t.Fatalf("got %#v, want nil", err)
		}

		expectedConfig := []string{
			`ARRAY_NAME="gp_upgrade cluster"`,
			"SEG_PREFIX=seg.AAAAAAAAAAA.",
			"TRUSTED_SHELL=ssh",
			"HBA_HOSTNAMES=1",
		}
		if !reflect.DeepEqual(actualConfig, expectedConfig) {
			t.Errorf("got %v, want %v", actualConfig, expectedConfig)
		}
	})
}

func TestGetCheckpointSegmentsAndEncoding(t *testing.T) {
	type mockQuery struct {
		sql      string
		result   string
		expected string
	}

	// the mock query order must match the query order in GetCheckpointSegmentsAndEncoding
	cases := []struct {
		version semver.Version
		query   []mockQuery
	}{
		{
			semver.MustParse("5.0.0"),
			[]mockQuery{
				{
					"SELECT .*server.*",
					"UNICODE",
					"ENCODING=UNICODE",
				},
				{
					"SELECT .*checkpoint.*",
					"8",
					"CHECK_POINT_SEGMENTS=8",
				},
			},
		},
		{
			semver.MustParse("6.0.0"),
			[]mockQuery{
				{
					"SELECT .*server.*",
					"UNICODE",
					"ENCODING=UNICODE",
				},
				{
					"SELECT .*checkpoint.*",
					"8",
					"CHECK_POINT_SEGMENTS=8",
				},
			},
		},
		{
			semver.MustParse("7.0.0"),
			[]mockQuery{
				{
					"SELECT .*server.*",
					"UNICODE",
					"ENCODING=UNICODE",
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("successfully get the GUC values for %s", c.version.String()), func(t *testing.T) {
			dbConn, sqlMock := testhelper.CreateAndConnectMockDB(1)

			var expected []string
			for _, mock := range c.query {
				mockRow := sqlmock.NewRows([]string{"string"}).AddRow(driver.Value(mock.result))
				sqlMock.ExpectQuery(mock.sql).WillReturnRows(mockRow)
				expected = append(expected, mock.expected)
			}

			actual, err := GetCheckpointSegmentsAndEncoding([]string{}, c.version, dbConn)
			if err != nil {
				t.Fatalf("got %#v, want nil", err)
			}

			if !reflect.DeepEqual(actual, expected) {
				t.Errorf("got %v, want %v", actual, expected)
			}
		})
	}
}

func TestWriteSegmentArray(t *testing.T) {
	test := func(t *testing.T, intermediateTarget *greenplum.Cluster, expected []string) {
		t.Helper()

		actual, err := WriteSegmentArray([]string{}, intermediateTarget)
		if err != nil {
			t.Errorf("got %#v", err)
		}

		sort.Strings(actual)
		sort.Strings(expected)
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
		config := MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir_upgrade/seg-1", Role: "p", Port: 15433},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1_upgrade/seg1", Role: "p", Port: 15434},
			{ContentID: 1, DbID: 3, Hostname: "sdw2", DataDir: "/data/dbfast2_upgrade/seg2", Role: "p", Port: 15434},
		})

		test(t, config, []string{
			"QD_PRIMARY_ARRAY=mdw~mdw~15433~/data/qddir_upgrade/seg-1~1~-1",
			"declare -a PRIMARY_ARRAY=(",
			"\tsdw1~sdw1~15434~/data/dbfast1_upgrade/seg1~2~0",
			"\tsdw2~sdw2~15434~/data/dbfast2_upgrade/seg2~3~1",
			")",
		})
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
		if !errors.As(err, &actual) {
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
		if !errors.As(err, &actual) {
			t.Fatalf("got %#v, want ExitError", err)
		}

		if actual.ExitCode() != 1 {
			t.Errorf("got %d, want 1", actual.ExitCode())
		}
	})

	t.Run("suppresses most environment variables during execution", func(t *testing.T) {
		// Set up the test environment.
		cleanup := clearEnv(t)
		defer cleanup()

		env := map[string]string{
			// Allowed keys.
			//
			// This allowlist was chosen from a manual inspection of the 5X
			// gpinitsystem. (These environment variables are used for logging
			// purposes.)
			"HOME":    "/home/gpadmin",
			"USER":    "gpadmin",
			"LOGNAME": "gpadmin-logname",

			// Disallowed.
			"PATH":            "/some/incorrect/location",
			"LD_LIBRARY_PATH": "/other/bad/location",
		}

		for k, v := range env {
			if err := os.Setenv(k, v); err != nil {
				t.Fatalf("setting up test environment: %+v", err)
			}
		}

		// Capture the actual environment received by the gpinitsystem process.
		SetExecCommand(exectest.NewCommand(EnvironmentMain))
		defer ResetExecCommand()

		out := &stdoutBuffer{}
		err := RunInitsystemForTargetCluster(out, gpHome6, gpinitsystemConfigPath, version6)

		if err != nil {
			t.Fatalf("got error: %+v", err)
		}

		// Validate that we only got these allowed vars.
		envAllowed := map[string]bool{
			"HOME":    true,
			"USER":    true,
			"LOGNAME": true,
		}

		scanner := bufio.NewScanner(&out.Buffer)
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.SplitN(line, "=", 2)

			if len(parts) < 2 {
				t.Errorf("envvar %q not in KEY=VALUE format", line)
				continue
			}

			key, value := parts[0], parts[1]
			if ok := envAllowed[key]; !ok {
				t.Errorf("disallowed envvar %q was passed to gpinitsystem", line)
			}

			if value != env[key] {
				t.Errorf("envvar %q has value %q, want %q", key, value, env[key])
			}
		}

		if err := scanner.Err(); err != nil {
			t.Errorf("scanning initsystem output: %+v", err)
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
	testlog.SetupLogger()

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

func TestFilterEnv(t *testing.T) {
	cases := []struct {
		name       string
		initialEnv map[string]string
		selected   []string
		expected   []string // in sorted order
	}{
		{
			name:     "does not modify empty environment",
			selected: []string{"ENV"},
		},
		{
			name: "selects only specified keys",
			initialEnv: map[string]string{
				"ENV1": "one",
				"ENV2": "two",
				"ENV3": "three",
			},
			selected: []string{"ENV1", "ENV3"},
			expected: []string{"ENV1=one", "ENV3=three"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Clear and load the initial environment.
			cleanup := clearEnv(t)
			defer cleanup()

			for k, v := range c.initialEnv {
				if err := os.Setenv(k, v); err != nil {
					t.Fatal(err)
				}
			}

			actual := filterEnv(c.selected)
			sort.Strings(actual)

			if !reflect.DeepEqual(actual, c.expected) {
				t.Errorf("filterEnv(%q) = %q, want %q", c.selected, actual, c.expected)
			}
		})
	}
}

// clearEnv unsets every environment variable and returns a cleanup function
// that undoes its work.
func clearEnv(t *testing.T) (cleanup func()) {
	var cleanups []func()

	for _, pair := range os.Environ() {
		parts := strings.SplitN(pair, "=", 2)
		key := parts[0]

		// TODO: it's confusing that MustClearEnv runs os.Unsetenv and not
		// os.Clearenv.
		c := testutils.MustClearEnv(t, key)
		cleanups = append(cleanups, c)
	}

	return func() {
		for _, c := range cleanups {
			c()
		}
	}
}

// stdoutBuffer is a steps.OutStreams implementation that stores stdout only.
type stdoutBuffer struct {
	Buffer bytes.Buffer
}

func (s *stdoutBuffer) Stdout() io.Writer {
	return &s.Buffer
}

func (s *stdoutBuffer) Stderr() io.Writer {
	return ioutil.Discard
}
