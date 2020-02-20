package hub_test

import (
	"reflect"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	. "github.com/greenplum-db/gpupgrade/hub"
)

func finishMock(mock sqlmock.Sqlmock, t *testing.T) {
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("%v", err)
	}
}

func TestCheckClusterIsBalanced(t *testing.T) {

	t.Run("list segments dbids not in preferred role", func(t *testing.T) {

		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("couldn't create sqlmock: %v", err)
		}
		defer finishMock(mock, t)

		dbids := []int{2, 5}
		dbidRows := sqlmock.NewRows([]string{"dbid"})
		for _, dbid := range dbids {
			dbidRows.AddRow(dbid)
		}

		mock.ExpectQuery(DbidNotInBalancedStateQuery).
			WillReturnRows(dbidRows)

		resultingDbids, err := FindUnbalancedSegments(db)
		if err != nil {
			t.Errorf("returned %#v", err)
		}

		if !reflect.DeepEqual(resultingDbids, dbids) {
			t.Fatalf("got %#v, want %#v", resultingDbids, dbids)
		}
	})

	t.Run("all segments are in preferred role", func(t *testing.T) {

		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("couldn't create sqlmock: %v", err)
		}
		defer finishMock(mock, t)

		dbidRows := sqlmock.NewRows([]string{"dbid"})

		mock.ExpectQuery(DbidNotInBalancedStateQuery).
			WillReturnRows(dbidRows)

		resultingDbids, err := FindUnbalancedSegments(db)

		if err != nil {
			t.Fatalf("returned %#v", err)
		}

		if len(resultingDbids) > 0 {
			t.Fatalf("returned %d", resultingDbids)
		}
	})
}
