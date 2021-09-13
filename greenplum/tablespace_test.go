// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"bytes"
	"database/sql/driver"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/utils"
)

func TestGetTablespaces(t *testing.T) {
	cases := []struct {
		name           string
		rows           [][]driver.Value
		versionStr     string
		expectedTuples TablespaceTuples
		error          error
	}{
		{
			name: "successfully returns tablespace tuples from db",
			rows: [][]driver.Value{
				{1, 1234, "pg_default", "/tmp/pg_default_tablespace", 0},
				{2, 1235, "my_tablespace", "/tmp/my_tablespace", 1},
			},
			versionStr: "",
			expectedTuples: TablespaceTuples{{
				DbId: 1,
				Oid:  1234,
				Name: "pg_default",
				Info: TablespaceInfo{
					Location:    "/tmp/pg_default_tablespace",
					UserDefined: 0,
				},
			}, {
				DbId: 2,
				Oid:  1235,
				Name: "my_tablespace",
				Info: TablespaceInfo{
					Location:    "/tmp/my_tablespace",
					UserDefined: 1,
				},
			}},
			error: nil,
		},
		{
			name:           "not supported version",
			rows:           nil,
			versionStr:     "6.1.0",
			expectedTuples: nil,
			error:          errors.New("version not supported to retrieve tablespace information"),
		},
		{
			name:           "tablespace query execution failed",
			rows:           nil,
			versionStr:     "",
			expectedTuples: nil,
			error:          errors.New("tablespace query"),
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprint(c.name), func(t *testing.T) {
			conn, mock := testhelper.CreateAndConnectMockDB(1)
			defer conn.Close()
			var rows *sqlmock.Rows
			if c.rows != nil {
				// Set up the connection to return the expected rows.
				rows = sqlmock.NewRows([]string{"dbid", "oid", "name", "location", "userdefined"})
				for _, row := range c.rows {
					rows.AddRow(row...)
				}

				mock.ExpectQuery("SELECT (.*)").WillReturnRows(rows)
				defer func() {
					if err := mock.ExpectationsWereMet(); err != nil {
						t.Errorf("%v", err)
					}
				}()
			}

			if c.versionStr != "" {
				testhelper.SetDBVersion(conn, c.versionStr)
			}

			results, err := GetTablespaceTuples(conn)
			if c.error != nil && !strings.Contains(err.Error(), c.error.Error()) {
				t.Errorf("got %+v, want %+v", err, c.error)
			}

			if !reflect.DeepEqual(results, c.expectedTuples) {
				t.Errorf("got configuration %+v, want %+v", results, c.expectedTuples)
			}
		})
	}
}

func TestNewTablespaces(t *testing.T) {
	cases := []struct {
		name     string
		tuples   TablespaceTuples
		expected Tablespaces
	}{
		{
			name: "only default tablespace",
			tuples: TablespaceTuples{
				{
					DbId: 1,
					Oid:  1663,
					Name: "pg_default",
					Info: TablespaceInfo{
						Location:    "/tmp/master/gpseg-1",
						UserDefined: 0,
					},
				},
				{
					DbId: 2,
					Oid:  1663,
					Name: "pg_default",
					Info: TablespaceInfo{
						Location:    "/tmp/primary/gpseg-1",
						UserDefined: 0,
					},
				},
			},
			expected: map[int]SegmentTablespaces{
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
			tuples: TablespaceTuples{
				{
					DbId: 1,
					Oid:  1663,
					Name: "pg_default",
					Info: TablespaceInfo{
						Location:    "/tmp/master/gpseg-1",
						UserDefined: 0,
					},
				},
				{
					DbId: 1,
					Oid:  1664,
					Name: "my_tablespace",
					Info: TablespaceInfo{
						Location:    "/tmp/master/1664",
						UserDefined: 1,
					},
				},
				{
					DbId: 2,
					Oid:  1663,
					Name: "pg_default",
					Info: TablespaceInfo{
						Location:    "/tmp/primary/gpseg0",
						UserDefined: 0,
					},
				},
				{
					DbId: 2,
					Oid:  1664,
					Name: "my_tablespace",
					Info: TablespaceInfo{
						Location:    "/tmp/primary/1664",
						UserDefined: 1,
					},
				},
			},
			expected: map[int]SegmentTablespaces{
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
			if got := NewTablespaces(c.tuples); !reflect.DeepEqual(got, c.expected) {
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
		connErr := errors.New("connection failed")
		conn := dbconn.NewDBConnFromEnvironment("testdb")
		conn.Driver = testhelper.TestDriver{ErrToReturn: connErr}

		tablespaces, err := TablespacesFromDB(conn, "")

		if err == nil {
			t.Errorf("Expected an error, but got nil")
		}
		if tablespaces != nil {
			t.Errorf("Expected tablespaces to be nil, but got %#v", tablespaces)
		}
		if !strings.Contains(err.Error(), connErr.Error()) {
			t.Errorf("Expected error: %+v, got: %+v", connErr.Error(), err.Error())
		}
	})

	t.Run("returns an error if the tablespace query fails", func(t *testing.T) {
		conn, mock := testhelper.CreateMockDBConn()
		testhelper.ExpectVersionQuery(mock, "5.3.4")

		queryErr := errors.New("failed to get tablespace information")
		mock.ExpectQuery("SELECT .* upgrade_tablespace").WillReturnError(queryErr)

		tablespaces, err := TablespacesFromDB(conn, "")

		if err == nil {
			t.Errorf("Expected an error, but got nil")
		}
		if tablespaces != nil {
			t.Errorf("Expected tablespaces to be nil, got %#v", tablespaces)
		}
		if !strings.Contains(err.Error(), queryErr.Error()) {
			t.Errorf("Expected error: %+v, got: %+v", queryErr.Error(), err.Error())
		}
	})

	t.Run("populates Tablespaces using DB information", func(t *testing.T) {

		conn, mock := testhelper.CreateMockDBConn()

		testhelper.ExpectVersionQuery(mock, "5.3.4")
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
		tablespaces, err := TablespacesFromDB(conn, expectedFileName)

		if err != nil {
			t.Errorf("got unexpected error: %+v", err)
		}

		if !createCalled {
			t.Error("expected Create() to be invoked")
		}

		if inputFileName != expectedFileName {
			t.Errorf("Create() got %q, want %q", inputFileName, expectedFileName)
		}

		expectedTablespaces := Tablespaces{
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

		conn, mock := testhelper.CreateMockDBConn()

		testhelper.ExpectVersionQuery(mock, "5.3.4")
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
		_, err := TablespacesFromDB(conn, expectedFileName)

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
		tuples   TablespaceTuples
		expected string
	}{
		{
			name: "successfully writes to buffer",
			tuples: TablespaceTuples{
				Tablespace{
					DbId: 1,
					Oid:  1663,
					Name: "default",
					Info: TablespaceInfo{
						"/tmp/master/gpseg-1",
						0,
					},
				},
				Tablespace{
					DbId: 2,
					Oid:  1664,
					Name: "my_tablespace",
					Info: TablespaceInfo{
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
