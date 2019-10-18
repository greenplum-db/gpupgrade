package services

import (
	"database/sql/driver"
	"testing"

	"github.com/golang/mock/gomock"
	"gopkg.in/DATA-DOG/go-sqlmock.v1"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
)

func TestCheckObjectCount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	testhelper.SetupTestLogger() // initialize gplog

	dbConn, sqlMock := testhelper.CreateAndConnectMockDB(1)

	fakeResults := sqlmock.NewRows([]string{"count"}).
		AddRow([]driver.Value{int32(2)}...)
	sqlMock.ExpectQuery(".*c.relstorage IN.*").
		WillReturnRows(fakeResults)

	fakeResults = sqlmock.NewRows([]string{"count"}).
		AddRow([]driver.Value{int32(3)}...)
	sqlMock.ExpectQuery(".*c.relstorage NOT IN.*").
		WillReturnRows(fakeResults)

	aocount, heapcount, err := GetCountsForDb(dbConn)
	if err != nil {
		t.Errorf("getting object counts failed: #%v", err)
	}

	if aocount != 2 {
		t.Errorf("wanted: 2 got: %d", aocount)
	}

	if heapcount != 3 {
		t.Errorf("wanted: 3 got: %d", heapcount)
	}
}
