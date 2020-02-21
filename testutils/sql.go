package testutils

import (
	"fmt"

	"github.com/greenplum-db/gpupgrade/utils"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
)

// MockSegmentConfiguration returns a set of sqlmock.Rows that contains the
// expected response to a gp_segment_configuration query.
//
// When changing this implementation, make sure you change MockCluster() to
// match!
func MockSegmentConfiguration() *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"dbid", "contentid", "port", "hostname", "datadir", "role", "preferredrole"})
	rows.AddRow(1, -1, 15432, "mdw", "/data/master/gpseg-1", "p", "p")
	rows.AddRow(2, 0, 25432, "sdw1", "/data/primary/gpseg0", "p", "p")

	return rows
}

// MockCluster returns the Cluster equivalent of MockSegmentConfiguration().
//
// When changing this implementation, make sure you change
// MockSegmentConfiguration() to match!
func MockCluster() *utils.Cluster {
	c, err := utils.NewCluster([]utils.SegConfig{
		{DbID: 1, ContentID: -1, Port: 15432, Hostname: "mdw", DataDir: "/data/master/gpseg-1", Role: "p", PreferredRole: "p"},
		{DbID: 2, ContentID: 0, Port: 25432, Hostname: "sdw1", DataDir: "/data/primary/gpseg0", Role: "p", PreferredRole: "p"},
	})

	if err != nil {
		panic(fmt.Sprintf("unexpected error %+v", err))
	}

	return c
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
