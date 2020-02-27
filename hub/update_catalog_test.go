package hub_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/greenplum-db/gpupgrade/hub"
)

// Sentinel error values to make error case testing easier.
var (
	ErrSentinel = fmt.Errorf("sentinel error")
	ErrRollback = fmt.Errorf("rollback failed")
)

// finishMock is a defer function to make the sqlmock API a little bit more like
// gomock. Use it like this:
//
//     db, mock, err := sqlmock.New()
//     if err != nil {
//         t.Fatalf("couldn't create sqlmock: %v", err)
//     }
//     defer finishMock(mock, t)
//
func finishMock(mock sqlmock.Sqlmock, t *testing.T) {
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("%v", err)
	}
}

func TestUpdateCatalog(t *testing.T) {
	src, err := utils.NewCluster([]utils.SegConfig{
		{ContentID: -1, Port: 123, Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
		{ContentID: -1, Port: 789, Role: utils.MirrorRole, PreferredRole: utils.MirrorRole},
		{ContentID: 0, Port: 234, Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
		{ContentID: 1, Port: 345, Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
		{ContentID: 2, Port: 456, Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
	})

	if err != nil {
		t.Fatalf("constructing test cluster: %+v", err)
	}

	tempDir, err := ioutil.TempDir("", "gpupgrade")
	if err != nil {
		t.Fatalf("creating temporary directory: %#v", err)
	}
	defer os.RemoveAll(tempDir)

	oldStateDir, isSet := os.LookupEnv("GPUGRADE_HOME")
	defer func() {
		if isSet {
			os.Setenv("GPUPGRADE_HOME", oldStateDir)
		}
	}()

	err = os.Setenv("GPUPGRADE_HOME", tempDir)
	if err != nil {
		t.Fatalf("failed to set GPUPGRADE_HOME %#v", err)
	}

	config := filepath.Join(tempDir, ConfigFileName)
	data := `{
	"Source": {
		"BinDir": "/usr/local/gpdb5/bin",
			"Version": {
			  "VersionString": "5.0.0",
			  "SemVer": "5.0.0"
			}
		},
	"Target": {
		"BinDir": "/usr/local/gpdb6/bin",
			"Version": {
			  "VersionString": "6.0.0-beta.1 build dev",
			  "SemVer": "6.0.0"
			}
		}
}`
	err = ioutil.WriteFile(config, []byte(data), 0600)
	if err != nil {
		t.Fatalf("creating %s: %+v", config, err)
	}

	conf := &Config{src, &utils.Cluster{}, InitializeConfig{}, 0, port, useLinkMode}
	server := New(conf, nil, tempDir)

	t.Run("updates ports for every segment", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("couldn't create sqlmock: %v", err)
		}
		defer finishMock(mock, t)

		contents := sqlmock.NewRows([]string{"content"})
		for _, content := range src.ContentIDs {
			contents.AddRow(content)
		}

		//XXX: this will ignore the begin/commit statement order
		mock.MatchExpectationsInOrder(false)

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT content FROM gp_segment_configuration").
			WillReturnRows(contents)

		// We want one port update for every segment.
		// Note that ranging over a map doesn't guarantee execution order, so we
		// range over the contents instead.
		for _, content := range src.ContentIDs {
			seg := src.Primaries[content]
			expectCatalogUpdate(mock, seg).
				WillReturnResult(sqlmock.NewResult(0, 1))

			// TODO: Uncomment it when target cluster handle mirror and standby
			// port and data directory renaming.
			// See the corresponding section in update_catalog.go
			//if mirror, ok := src.Mirrors[content]; ok {
			//	expectCatalogUpdate(mock, mirror).
			//		WillReturnResult(sqlmock.NewResult(0, 1))
			//}
		}

		mock.ExpectCommit()

		err = server.UpdateGpSegmentConfiguration(db)
		if err != nil {
			t.Errorf("returned error %+v", err)
		}
	})

	// All cases added to this table expect an error to be returned. The core of
	// the test case is .prepare(), which sets up the Sqlmock appropriately.
	// The case's .check() method tests that the returned error is what the test
	// case expected; in simple cases you can use the match() helper to check
	// that the error is of a particular "type".
	errorCases := []struct {
		name    string
		prepare func(sqlmock.Sqlmock)
		verify  func(*testing.T, error)
	}{{
		"on transaction failure",
		func(mock sqlmock.Sqlmock) {
			mock.ExpectBegin().WillReturnError(ErrSentinel)
		},
		expect(ErrSentinel),
	}, {
		"and rolls back on query failure",
		func(mock sqlmock.Sqlmock) {
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT").WillReturnError(ErrSentinel)
			mock.ExpectRollback()
		},
		expect(ErrSentinel),
	}, {
		"and rolls back on iteration failure",
		func(mock sqlmock.Sqlmock) {
			contents := sqlmock.NewRows([]string{"content"}).
				AddRow(-1).
				RowError(0, ErrSentinel)

			mock.ExpectBegin()
			mock.ExpectQuery("SELECT content FROM gp_segment_configuration").
				WillReturnRows(contents)
			mock.ExpectRollback()
		},
		expect(ErrSentinel),
	}, {
		"and rolls back on update failure",
		func(mock sqlmock.Sqlmock) {
			contents := sqlmock.NewRows([]string{"content"})
			for _, content := range src.ContentIDs {
				contents.AddRow(content)
			}

			mock.ExpectBegin()
			mock.ExpectQuery("SELECT content FROM gp_segment_configuration").
				WillReturnRows(contents)
			expectCatalogUpdate(mock, src.Primaries[-1]).
				WillReturnError(ErrSentinel)
			mock.ExpectRollback()
		},
		expect(ErrSentinel),
	}, {
		"on commit failure",
		func(mock sqlmock.Sqlmock) {
			contents := sqlmock.NewRows([]string{"content"})
			for _, content := range src.ContentIDs {
				contents.AddRow(content)
			}

			mock.MatchExpectationsInOrder(false) // XXX see above

			mock.ExpectBegin()
			mock.ExpectQuery("SELECT content FROM gp_segment_configuration").
				WillReturnRows(contents)

			for _, content := range src.ContentIDs {
				seg := src.Primaries[content]
				expectCatalogUpdate(mock, seg).
					WillReturnResult(sqlmock.NewResult(0, 1))

				// XXX See above
				//if mirror, ok := src.Mirrors[content]; ok {
				//	expectCatalogUpdate(mock, mirror).
				//		WillReturnResult(sqlmock.NewResult(0, 1))
				//}
			}

			mock.ExpectCommit().WillReturnError(ErrSentinel)
		},
		expect(ErrSentinel),
	}, {
		"and rolls back on row scan failure",
		func(mock sqlmock.Sqlmock) {
			contents := sqlmock.NewRows([]string{"content"}).
				AddRow("hello")

			mock.ExpectBegin()
			mock.ExpectQuery("SELECT content FROM gp_segment_configuration").
				WillReturnRows(contents)
			mock.ExpectRollback()
		},
		func(t *testing.T, err error) {
			// XXX It'd be nice to inject our ErrSentinel into the Scan
			// function, but there doesn't seem to be a way to do that. We
			// instead return junk values from our query, and search for the
			// "scan error" hardcoded string. Blech.
			if err == nil || !strings.Contains(err.Error(), "Scan error") {
				t.Errorf("returned %#v which does not appear to be a scan error", err)
			}
		},
	}, {
		"on rollback",
		func(mock sqlmock.Sqlmock) {
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT").WillReturnError(ErrSentinel)
			mock.ExpectRollback().WillReturnError(ErrRollback)
		},
		func(t *testing.T, err error) {
			multiErr, ok := err.(*multierror.Error)
			if !ok {
				t.Fatal("did not return a multierror")
			}
			if !xerrors.Is(multiErr.Errors[0], ErrSentinel) {
				t.Errorf("first error was %#v want %#v", err, ErrSentinel)
			}
			if !xerrors.Is(multiErr.Errors[1], ErrRollback) {
				t.Errorf("second error was %#v want %#v", err, ErrRollback)
			}
		},
	}, {
		"when there are content ids in the database missing from the cluster",
		func(mock sqlmock.Sqlmock) {
			contents := sqlmock.NewRows([]string{"content"}).
				AddRow(-1).
				AddRow(0).
				AddRow(1).
				AddRow(2).
				AddRow(3) // extra content, does not exist

			mock.ExpectBegin()
			mock.ExpectQuery("SELECT content FROM gp_segment_configuration").
				WillReturnRows(contents)
			mock.ExpectRollback()
		},
		expect(ErrContentMismatch),
	}, {
		"when there are content ids in the cluster missing from the database",
		func(mock sqlmock.Sqlmock) {
			contents := sqlmock.NewRows([]string{"content"}).
				AddRow(-1).
				AddRow(0).
				// missing content, skipping 1
				AddRow(2)

			mock.ExpectBegin()
			mock.ExpectQuery("SELECT content FROM gp_segment_configuration").
				WillReturnRows(contents)
			mock.ExpectRollback()
		},
		expect(ErrContentMismatch),
	}, {
		"if UPDATE updates multiple rows",
		func(mock sqlmock.Sqlmock) {
			contents := sqlmock.NewRows([]string{"content"}).
				AddRow(-1).
				AddRow(0).
				AddRow(1).
				AddRow(2)

			mock.ExpectBegin()
			mock.ExpectQuery("SELECT content FROM gp_segment_configuration").
				WillReturnRows(contents)

			expectCatalogUpdate(mock, src.Primaries[-1]).
				WillReturnResult(sqlmock.NewResult(0, 2))

			mock.ExpectRollback()
		},
		func(t *testing.T, err error) {
			expectedErr := "updated 2 rows for content -1, expected 1"
			if err.Error() != expectedErr {
				t.Errorf("returned '%s' want '%s'", err.Error(), expectedErr)
			}
		},
	}}

	for _, c := range errorCases {
		t.Run(fmt.Sprintf("returns an error %s", c.name), func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("couldn't create sqlmock: %v", err)
			}
			defer finishMock(mock, t)

			// prepare() sets up any mock expectations.
			c.prepare(mock)

			err = server.UpdateGpSegmentConfiguration(db)

			// Make sure the error is the one we expect.
			c.verify(t, err)
		})
	}
}

// expect is a helper for the errorCases table test above. You can use it as the
// errorCase.verify() callback implementation; it just uses errors.Is() to see
// whether the actual error returned matches the expected one, and complains
// through the testing.T if not.
func expect(expected error) func(*testing.T, error) {
	return func(t *testing.T, actual error) {
		if !xerrors.Is(actual, expected) {
			t.Errorf("returned %#v want %#v", actual, expected)
		}
	}
}

// expectCatalogUpdate is here so we don't have to copy-paste the expected UPDATE
// statement everywhere.
func expectCatalogUpdate(mock sqlmock.Sqlmock, seg utils.SegConfig) *sqlmock.ExpectedExec {
	return mock.ExpectExec(
		"UPDATE gp_segment_configuration SET port = (.+), datadir = (.+) WHERE content = (.+) AND role = (.+)",
	).WithArgs(seg.Port, seg.DataDir, seg.ContentID, seg.Role)
}
