package hub_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/utils"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	multierror "github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

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

func TestClonePortsFromCluster(t *testing.T) {
	src, err := utils.NewCluster([]utils.SegConfig{
		{ContentID: -1, Port: 123, Role: "p"},
		{ContentID: 0, Port: 234, Role: "p"},
		{ContentID: 1, Port: 345, Role: "p"},
		{ContentID: 2, Port: 456, Role: "p"},
	})
	if err != nil {
		t.Fatalf("constructing test cluster: %+v", err)
	}

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

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT content FROM gp_segment_configuration").
			WillReturnRows(contents)

		// We want one port update for every segment.
		// Note that ranging over a map doesn't guarantee execution order, so we
		// range over the contents instead.
		for _, content := range src.ContentIDs {
			conf := src.Primaries[content]
			mock.ExpectExec("UPDATE gp_segment_configuration SET port = (.+) WHERE content = (.+)").
				WithArgs(conf.Port, content).
				WillReturnResult(sqlmock.NewResult(0, 1))
		}

		mock.ExpectCommit()

		err = ClonePortsFromCluster(db, src)
		if err != nil {
			t.Fatalf("returned error %#v", err)
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
			mock.ExpectExec("UPDATE gp_segment_configuration SET port = (.+) WHERE content = (.+)").
				WithArgs(src.Primaries[-1].Port, -1).
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

			mock.ExpectBegin()
			mock.ExpectQuery("SELECT content FROM gp_segment_configuration").
				WillReturnRows(contents)

			for _, content := range src.ContentIDs {
				conf := src.Primaries[content]
				mock.ExpectExec("UPDATE gp_segment_configuration SET port = (.+) WHERE content = (.+)").
					WithArgs(conf.Port, content).
					WillReturnResult(sqlmock.NewResult(0, 1))
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

			mock.ExpectExec("UPDATE gp_segment_configuration SET port = (.+) WHERE content = (.+)").
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

			err = ClonePortsFromCluster(db, src)

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
