// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum_test

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestGetTablespaces(t *testing.T) {
	t.Run("retrieves tablespaces", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("couldn't create sqlmock: %v", err)
		}
		defer testutils.FinishMock(mock, t)
		defer db.Close()

		rows := sqlmock.NewRows([]string{"dbid", "oid", "name", "location", "userdefined"})
		rows.AddRow(1, 1234, "pg_default", "/tmp/pg_default_tablespace", 0)
		rows.AddRow(2, 1235, "my_tablespace", "/tmp/my_tablespace", 1)

		mock.ExpectQuery("SELECT").WillReturnRows(rows)

		actual, err := greenplum.GetTablespaceTuples(db)
		if err != nil {
			t.Errorf("returned error %+v", err)
		}

		expected := greenplum.TablespaceTuples{
			greenplum.Tablespace{
				DbId: 1,
				Oid:  1234,
				Name: "pg_default",
				Info: greenplum.TablespaceInfo{Location: "/tmp/pg_default_tablespace", UserDefined: 0},
			},
			greenplum.Tablespace{
				DbId: 2,
				Oid:  1235,
				Name: "my_tablespace",
				Info: greenplum.TablespaceInfo{Location: "/tmp/my_tablespace", UserDefined: 1},
			},
		}

		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("got configuration %+v, want %+v", actual, expected)
		}
	})

	// error cases
	expectedErr := errors.New("tablespace query")

	errorCases := []struct {
		name    string
		version semver.Version
		error   error
	}{
		{
			name:    " query execution failed",
			version: semver.MustParse("6.0.0"),
			error:   expectedErr,
		},
		{
			name:    "tablespace query execution failed",
			version: semver.MustParse("6.0.0"),
			error:   expectedErr,
		},
	}

	for _, c := range errorCases {
		t.Run(c.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("couldn't create sqlmock: %v", err)
			}
			defer testutils.FinishMock(mock, t)
			defer db.Close()

			mock.ExpectQuery("SELECT").WillReturnError(c.error)

			tuples, err := greenplum.GetTablespaceTuples(db)
			if !errors.Is(err, c.error) {
				t.Errorf("returned %#v want %#v", err, c.error)
			}

			if tuples != nil {
				t.Errorf("unexpected results %+v", tuples)
			}
		})
	}
}

func TestNewTablespaces(t *testing.T) {
	cases := []struct {
		name     string
		tuples   greenplum.TablespaceTuples
		expected greenplum.Tablespaces
	}{
		{
			name: "only default tablespace",
			tuples: greenplum.TablespaceTuples{
				{
					DbId: 1,
					Oid:  1663,
					Name: "pg_default",
					Info: greenplum.TablespaceInfo{
						Location:    "/tmp/master/gpseg-1",
						UserDefined: 0,
					},
				},
				{
					DbId: 2,
					Oid:  1663,
					Name: "pg_default",
					Info: greenplum.TablespaceInfo{
						Location:    "/tmp/primary/gpseg-1",
						UserDefined: 0,
					},
				},
			},
			expected: map[int]greenplum.SegmentTablespaces{
				1: {
					1663: {
						Location:    "/tmp/master/gpseg-1",
						UserDefined: 0,
					},
				},
				2: {
					1663: {
						Location:    "/tmp/primary/gpseg-1",
						UserDefined: 0,
					},
				},
			},
		},
		{
			name: "multiple tablespaces",
			tuples: greenplum.TablespaceTuples{
				{
					DbId: 1,
					Oid:  1663,
					Name: "pg_default",
					Info: greenplum.TablespaceInfo{
						Location:    "/tmp/master/gpseg-1",
						UserDefined: 0,
					},
				},
				{
					DbId: 1,
					Oid:  1664,
					Name: "my_tablespace",
					Info: greenplum.TablespaceInfo{
						Location:    "/tmp/master/1664",
						UserDefined: 1,
					},
				},
				{
					DbId: 2,
					Oid:  1663,
					Name: "pg_default",
					Info: greenplum.TablespaceInfo{
						Location:    "/tmp/primary/gpseg0",
						UserDefined: 0,
					},
				},
				{
					DbId: 2,
					Oid:  1664,
					Name: "my_tablespace",
					Info: greenplum.TablespaceInfo{
						Location:    "/tmp/primary/1664",
						UserDefined: 1,
					},
				},
			},
			expected: map[int]greenplum.SegmentTablespaces{
				1: {
					1663: {
						Location:    "/tmp/master/gpseg-1",
						UserDefined: 0,
					},
					1664: {
						Location:    "/tmp/master/1664",
						UserDefined: 1,
					},
				},
				2: {
					1663: {
						Location:    "/tmp/primary/gpseg0",
						UserDefined: 0,
					},
					1664: {
						Location:    "/tmp/primary/1664",
						UserDefined: 1,
					},
				},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := greenplum.NewTablespaces(c.tuples); !reflect.DeepEqual(got, c.expected) {
				t.Errorf("NewTablespaces() = %v, want %v", got, c.expected)
			}
		})
	}
}

// returns a set of sqlmock.Rows that contains the expected
// response to a tablespace query.
func MockTablespaceQueryResult() *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"dbid", "oid", "name", "location", "userdefined"})
	rows.AddRow(1, 1663, "pg_default", "/tmp/master_tablespace", 0)
	rows.AddRow(2, 1663, "pg_default", "/tmp/my_tablespace", 0)

	return rows
}

func TestTablespacesFromDB(t *testing.T) {
	t.Run("returns an error if connection fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("couldn't create sqlmock: %v", err)
		}
		defer testutils.FinishMock(mock, t)

		expected := errors.New("connection failed")
		mock.ExpectQuery("SELECT").WillReturnError(expected)

		tablespaces, err := greenplum.TablespacesFromDB(db, "")
		if !errors.Is(err, expected) {
			t.Errorf("got %#v want %#v", err, expected)
		}

		if tablespaces != nil {
			t.Errorf("Expected tablespaces to be nil, but got %#v", tablespaces)
		}
	})

	t.Run("returns an error if the tablespace query fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("couldn't create sqlmock: %v", err)
		}
		defer testutils.FinishMock(mock, t)

		expected := errors.New("failed to get tablespace information")
		mock.ExpectQuery("SELECT .* upgrade_tablespace").WillReturnError(expected)

		tablespaces, err := greenplum.TablespacesFromDB(db, "")

		if err == nil {
			t.Errorf("Expected an error, but got nil")
		}
		if tablespaces != nil {
			t.Errorf("Expected tablespaces to be nil, got %#v", tablespaces)
		}
		if !strings.Contains(err.Error(), expected.Error()) {
			t.Errorf("Expected error: %+v, got: %+v", expected.Error(), err.Error())
		}
	})

	t.Run("populates Tablespaces using DB information", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("couldn't create sqlmock: %v", err)
		}
		defer testutils.FinishMock(mock, t)

		mock.ExpectQuery("SELECT .* upgrade_tablespace").WillReturnRows(MockTablespaceQueryResult())

		_, write, _ := os.Pipe()
		createCalled := false
		inputFileName := ""
		utils.System.Create = func(name string) (*os.File, error) {
			inputFileName = name
			createCalled = true
			return write, nil
		}
		defer write.Close()

		expectedFileName := "/tmp/mappingFile.txt"
		tablespaces, err := greenplum.TablespacesFromDB(db, expectedFileName)

		if err != nil {
			t.Errorf("got unexpected error: %+v", err)
		}

		if !createCalled {
			t.Error("expected Create() to be invoked")
		}

		if inputFileName != expectedFileName {
			t.Errorf("Create() got %q, want %q", inputFileName, expectedFileName)
		}

		expectedTablespaces := greenplum.Tablespaces{
			1: {
				1663: {
					Location:    "/tmp/master_tablespace",
					UserDefined: 0,
				},
			},
			2: {
				1663: {
					Location:    "/tmp/my_tablespace",
					UserDefined: 0,
				},
			},
		}

		if !reflect.DeepEqual(tablespaces, expectedTablespaces) {
			t.Errorf("expected: %#v got: %#v", expectedTablespaces, tablespaces)
		}
	})

	t.Run("fails to create tablespacesFile", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("couldn't create sqlmock: %v", err)
		}
		defer testutils.FinishMock(mock, t)

		mock.ExpectQuery("SELECT .* upgrade_tablespace").WillReturnRows(MockTablespaceQueryResult())

		expectedFileName := "/tmp/mappingFile.txt"
		expectedError := errors.New("permission denied")
		createCalled := false
		utils.System.Create = func(name string) (*os.File, error) {
			if name != expectedFileName {
				t.Errorf("got %q, want %q", name, expectedFileName)
			}
			createCalled = true
			return nil, expectedError
		}
		_, err = greenplum.TablespacesFromDB(db, expectedFileName)

		if err == nil {
			t.Errorf("expected error: %+v", expectedError)
		}

		if !createCalled {
			t.Errorf("expected Create() to be called")
		}
	})
}

func TestWrite(t *testing.T) {
	tests := []struct {
		name     string
		tuples   greenplum.TablespaceTuples
		expected string
	}{
		{
			name: "successfully writes to buffer",
			tuples: greenplum.TablespaceTuples{
				greenplum.Tablespace{
					DbId: 1,
					Oid:  1663,
					Name: "default",
					Info: greenplum.TablespaceInfo{
						"/tmp/master/gpseg-1",
						0,
					},
				},
				greenplum.Tablespace{
					DbId: 2,
					Oid:  1664,
					Name: "my_tablespace",
					Info: greenplum.TablespaceInfo{
						"/tmp/master/gpseg-1",
						1,
					},
				},
			},
			expected: "1,1663,default,/tmp/master/gpseg-1,0\n2,1664,my_tablespace,/tmp/master/gpseg-1,1\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w := &bytes.Buffer{}
			err := test.tuples.Write(w)
			if err != nil {
				t.Errorf("Write() got error %v", err)
			}
			if data := w.String(); w.String() != test.expected {
				t.Errorf("Write() gotW = %v, want %v", data, test.expected)
			}
		})
	}
}
