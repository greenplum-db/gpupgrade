package hub

import (
	"bytes"
	"database/sql"
	"errors"
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
)

type greenplumStub struct {
	run func(utilityName string, arguments ...string) error
}

func (g *greenplumStub) Run(utilityName string, arguments ...string) error {
	return g.run(utilityName, arguments...)
}

func TestWriteGpAddmirrorsConfig(t *testing.T) {
	t.Run("streams the gpaddmirrors config file format", func(t *testing.T) {
		mirrors := []greenplum.SegConfig{
			{
				DbID:      3,
				ContentID: 0,
				Port:      234,
				Hostname:  "localhost",
				DataDir:   "/data/mirrors_upgrade/seg0",
				Role:      "m",
			},
			{
				DbID:      4,
				ContentID: 1,
				Port:      235,
				Hostname:  "localhost",
				DataDir:   "/data/mirrors_upgrade/seg1",
				Role:      "m",
			},
		}
		var out bytes.Buffer

		err := writeGpAddmirrorsConfig(mirrors, &out)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		lines := []string{
			"0|localhost|234|/data/mirrors_upgrade/seg0",
			"1|localhost|235|/data/mirrors_upgrade/seg1",
		}

		expected := strings.Join(lines, "\n") + "\n"

		if out.String() != expected {
			t.Errorf("got %q want %q", out.String(), expected)
		}
	})

	t.Run("returns errors from provided write stream", func(t *testing.T) {
		mirrors := []greenplum.SegConfig{
			{DbID: 3, ContentID: 0, Port: 234, Hostname: "localhost", DataDir: "/data/mirrors/seg0", Role: "m"},
		}

		writer := &failingWriter{errors.New("ahhh")}

		err := writeGpAddmirrorsConfig(mirrors, writer)
		if !xerrors.Is(err, writer.err) {
			t.Errorf("returned error %#v, want %#v", err, writer.err)
		}
	})
}

func TestRunAddMirrors(t *testing.T) {
	t.Run("runs gpaddmirrors with the created config file", func(t *testing.T) {
		expectedFilepath := "/add/mirrors/config_file"
		runCalled := false

		stub := &greenplumStub{
			func(utility string, arguments ...string) error {
				runCalled = true

				expectedUtility := "gpaddmirrors"
				if utility != expectedUtility {
					t.Errorf("ran utility %q, want %q", utility, expectedUtility)
				}

				var fs flag.FlagSet

				actualFilepath := fs.String("i", "", "")
				quietMode := fs.Bool("a", false, "")

				err := fs.Parse(arguments)
				if err != nil {
					t.Fatalf("error parsing arguments: %+v", err)
				}

				if *actualFilepath != expectedFilepath {
					t.Errorf("got filepath %q, want %q", *actualFilepath, expectedFilepath)
				}

				if !*quietMode {
					t.Errorf("missing -a flag")
				}
				return nil
			},
		}

		err := runAddMirrors(stub, expectedFilepath)
		if err != nil {
			t.Errorf("returned error %+v", err)
		}

		if !runCalled {
			t.Errorf("GreenplumRunner.Run() was not called")
		}
	})

	t.Run("bubbles up errors from the utility", func(t *testing.T) {
		stub := new(greenplumStub)

		expected := errors.New("ahhhh")
		stub.run = func(_ string, _ ...string) error {
			return expected
		}

		actual := runAddMirrors(stub, "")
		if !xerrors.Is(actual, expected) {
			t.Errorf("returned error %#v, want %#v", actual, expected)
		}
	})
}

func TestDoUpgrade(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("couldn't create sqlmock: %v", err)
	}
	defer testutils.FinishMock(mock, t)

	t.Run("writes the add mirrors config to the and runs add mirrors", func(t *testing.T) {
		stateDir := "/the/state/dir"
		expectedFilepath := filepath.Join(stateDir, "add_mirrors_config")
		runCalled := false
		readPipe, writePipe, err := os.Pipe()
		if err != nil {
			t.Errorf("error creating pipes %#v", err)
		}

		utils.System.Create = func(name string) (*os.File, error) {
			if name != expectedFilepath {
				t.Errorf("got filepath %q want %q", name, expectedFilepath)
			}
			if err != nil {
				return nil, err
			}
			return writePipe, nil
		}

		mirrors := []greenplum.SegConfig{
			{
				DbID:      3,
				ContentID: 0,
				Port:      234,
				Hostname:  "localhost",
				DataDir:   "/data/mirrors_upgrade/seg0",
				Role:      "m",
			},
			{
				DbID:      4,
				ContentID: 1,
				Port:      235,
				Hostname:  "localhost",
				DataDir:   "/data/mirrors_upgrade/seg1",
				Role:      "m",
			},
		}

		stub := greenplumStub{run: func(utilityName string, arguments ...string) error {
			runCalled = true

			expectedUtility := "gpaddmirrors"
			if utilityName != expectedUtility {
				t.Errorf("ran utility %q, want %q", utilityName, expectedUtility)
			}

			var fs flag.FlagSet

			actualFilepath := fs.String("i", "", "")
			quietMode := fs.Bool("a", false, "")

			err := fs.Parse(arguments)
			if err != nil {
				t.Fatalf("error parsing arguments: %+v", err)
			}

			if *actualFilepath != expectedFilepath {
				t.Errorf("got filepath %q want %q", *actualFilepath, expectedFilepath)
			}

			if !*quietMode {
				t.Errorf("missing -a flag")
			}
			return nil
		}}

		expectFtsProbe(mock)
		expectMirrorsAndReturn(mock, "t")

		err = doUpgrade(db, stateDir, mirrors, &stub)

		if err != nil {
			t.Errorf("got unexpected error from UpgradeMirrors %#v", err)
		}

		expectedLines := []string{
			"0|localhost|234|/data/mirrors_upgrade/seg0",
			"1|localhost|235|/data/mirrors_upgrade/seg1",
		}

		expectedFileContents := strings.Join(expectedLines, "\n") + "\n"
		fileContents, _ := ioutil.ReadAll(readPipe)

		if expectedFileContents != string(fileContents) {
			t.Errorf("got file contents %q want %q", fileContents, expectedFileContents)
		}

		if !runCalled {
			t.Errorf("GreenplumRunner.Run() was not called")
		}
	})

	t.Run("returns the error when create file path fails", func(t *testing.T) {
		expectedError := errors.New("i'm an error")
		utils.System.Create = func(name string) (file *os.File, err error) {
			return nil, expectedError
		}

		err = doUpgrade(db, "", []greenplum.SegConfig{}, &greenplumStub{})
		if !xerrors.Is(err, expectedError) {
			t.Errorf("returned error %#v want %#v", err, expectedError)
		}
	})

	t.Run("returns the error when writing and closing the config file fails", func(t *testing.T) {
		utils.System.Create = func(name string) (file *os.File, err error) {
			// A nil file will result in failure.
			return nil, nil
		}

		// We need at least one config entry to cause something to be written.
		mirrors := []greenplum.SegConfig{
			{DbID: 3, ContentID: 0, Port: 234, Hostname: "localhost", DataDir: "/data/mirrors/seg0", Role: "m"},
		}

		stub := new(greenplumStub)
		stub.run = func(_ string, _ ...string) error {
			t.Errorf("gpaddmirrors should not have been called")
			return nil
		}

		err = doUpgrade(db, "/state/dir", mirrors, stub)

		var merr *multierror.Error
		if !xerrors.As(err, &merr) {
			t.Fatalf("returned error %#v, want error type %T", err, merr)
		}

		if len(merr.Errors) != 2 {
			t.Errorf("expected exactly two errors")
		}

		for _, err := range merr.Errors {
			if !xerrors.Is(err, os.ErrInvalid) {
				t.Errorf("returned error %#v want %#v", err, os.ErrInvalid)
			}
		}
	})

	t.Run("returns the error when running the command fails", func(t *testing.T) {
		utils.System.Create = func(name string) (file *os.File, err error) {
			_, writePipe, _ := os.Pipe()
			return writePipe, nil
		}

		expected := errors.New("the error happened")
		stub := &greenplumStub{run: func(utilityName string, arguments ...string) error {
			return expected
		}}

		err = doUpgrade(db, "/state/dir", []greenplum.SegConfig{}, stub)
		if !xerrors.Is(err, expected) {
			t.Errorf("returned error %#v want %#v", err, expected)
		}
	})
}

func TestUpgradeMirrors(t *testing.T) {
	stub := &greenplumStub{
		func(utility string, arguments ...string) error {
			return nil
		},
	}

	t.Run("creates db connection with correct data source settings", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("couldn't create sqlmock: %v", err)
		}
		defer testutils.FinishMock(mock, t)

		_, writePipe, err := os.Pipe()
		utils.System.Create = func(name string) (*os.File, error) {
			return writePipe, nil
		}

		expectFtsProbe(mock)
		expectMirrorsAndReturn(mock, "t")

		utils.System.SqlOpen = func(driverName, dataSourceName string) (*sql.DB, error) {
			expected := "postgresql://localhost:123/template1?gp_session_role=utility&search_path="
			if dataSourceName != expected {
				t.Errorf("got: %q want: %q", dataSourceName, expected)
			}

			return db, nil
		}

		err = UpgradeMirrors("", 123, []greenplum.SegConfig{}, stub)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("returns error when failing to open db connection", func(t *testing.T) {
		expected := errors.New("failed to open db")
		utils.System.SqlOpen = func(driverName, dataSourceName string) (*sql.DB, error) {
			return nil, expected
		}

		err := UpgradeMirrors("", 123, []greenplum.SegConfig{}, stub)
		if !xerrors.Is(err, expected) {
			t.Errorf("got: %#v want: %#v", err, expected)
		}
	})
}

func TestWaitForFTS(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("couldn't create sqlmock: %v", err)
	}
	defer testutils.FinishMock(mock, t)

	t.Run("succeeds", func(t *testing.T) {
		expectFtsProbe(mock)
		expectMirrorsAndReturn(mock, "t")

		err = waitForFTS(db, defaultFTSTimeout)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("waits for mirrors to come up", func(t *testing.T) {
		expectFtsProbe(mock)
		expectMirrorsAndReturn(mock, "f")
		expectFtsProbe(mock)
		expectMirrorsAndReturn(mock, "t")

		err = waitForFTS(db, defaultFTSTimeout)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("times out if the mirrors never come up", func(t *testing.T) {
		expectFtsProbe(mock)
		expectMirrorsAndReturn(mock, "f")

		err = waitForFTS(db, -1*time.Second)

		expected := "-1s timeout exceeded waiting for mirrors to come up"
		if err.Error() != expected {
			t.Errorf("got: %#v want %s", err, expected)
		}
	})
}

func expectFtsProbe(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT gp_request_fts_probe_scan\(\);`).
		WillReturnRows(sqlmock.NewRows([]string{"gp_request_fts_probe_scan"}).AddRow("t"))
}

func expectMirrorsAndReturn(mock sqlmock.Sqlmock, up string) {
	mock.ExpectQuery(`SELECT every\(status = 'u' AND mode = 's'\)
							FROM gp_segment_configuration
							WHERE role = 'm'`).
		WillReturnRows(sqlmock.NewRows([]string{"every"}).AddRow(up))
}
