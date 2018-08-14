package testutils

import (
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	sqlmock "gopkg.in/DATA-DOG/go-sqlmock.v1"
)

// MockSegmentConfiguration returns a set of sqlmock.Rows that contains the
// expected response to a gp_segment_configuration query.
//
// When changing this implementation, make sure you change MockCluster() to
// match!
func MockSegmentConfiguration() *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"dbid", "contentid", "port", "hostname", "datadir"})
	rows.AddRow(1, -1, 15432, "mdw", "/data/master/gpseg-1")
	rows.AddRow(2, 0, 25432, "sdw1", "/data/primary/gpseg0")

	return rows
}

// MockCluster returns the Cluster equivalent of MockSegmentConfiguration().
//
// When changing this implementation, make sure you change
// MockSegmentConfiguration() to match!
func MockCluster() *cluster.Cluster {
	return &cluster.Cluster{
		ContentIDs: []int{-1, 0},
		Segments: map[int]cluster.SegConfig{
			-1: {DbID: 1, ContentID: -1, Port: 15432, Hostname: "mdw", DataDir: "/data/master/gpseg-1"},
			0:  {DbID: 2, ContentID: 0, Port: 25432, Hostname: "sdw1", DataDir: "/data/primary/gpseg0"},
		},
		Executor: &cluster.GPDBExecutor{},
	}
}

func SetMockGPDBVersion(mock sqlmock.Sqlmock, version string) {
	rows := sqlmock.NewRows([]string{"versionstring"})

	versionStr := fmt.Sprintf(`PostgreSQL 9.2beta2 (Greenplum Database %s-alpha.0+dev.9374.gfb2a077e20 build dev-oss)`,
		version)
	rows.AddRow(versionStr)

	mock.ExpectQuery(`SELECT version\(\) AS versionstring`).WillReturnRows(rows)
}

// CreateMockDBConn is just like testhelper.CreateAndConnectMockDB(), but it
// doesn't actually connect or set a version.
func CreateMockDBConn() (*dbconn.DBConn, sqlmock.Sqlmock) {
	mockdb, mock := testhelper.CreateMockDB()
	driver := testhelper.TestDriver{DB: mockdb, DBName: "testdb", User: "testrole"}
	connection := dbconn.NewDBConnFromEnvironment("testdb")
	connection.Driver = driver
	return connection, mock
}
