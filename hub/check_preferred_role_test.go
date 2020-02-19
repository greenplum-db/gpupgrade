package hub

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/xerrors"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	. "github.com/greenplum-db/gpupgrade/hub"
)

func finishMock(mock sqlmock.Sqlmock, t *testing.T) {
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("%v", err)
	}
}

func TestCheckSegmentNotInPreferredRole(t *testing.T) {

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

		mock.ExpectQuery("SELECT dbid FROM pg_catalog.gp_segment_configuration WHERE role != preferred_role").
			WillReturnRows(dbidRows)

		message := fmt.Sprintf(`
		Segment dbid %s are not in preferred role.
		All the segments must be in their preferred role for gpupgrade to continue. 
		Use gprecoverseg for bringing the segments in their preferred role.`,
			strings.Join(strings.Fields(fmt.Sprint(dbids)), ","))
		expectedError := xerrors.Errorf(message)

		err = CheckDbIdsNotInPreferredRole(db)

		if !reflect.DeepEqual(err.Error(), expectedError.Error()) {
			t.Fatalf("got %#v, want %#v", err, expectedError)
		}
	})

	t.Run("all segments are in preferred role", func(t *testing.T) {

		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("couldn't create sqlmock: %v", err)
		}
		defer finishMock(mock, t)

		dbidRows := sqlmock.NewRows([]string{"dbid"})

		mock.ExpectQuery("SELECT dbid FROM pg_catalog.gp_segment_configuration WHERE role != preferred_role").
			WillReturnRows(dbidRows)

		err = CheckDbIdsNotInPreferredRole(db)

		if err != nil {
			t.Fatalf("returned %#v", err)
		}
	})
}
