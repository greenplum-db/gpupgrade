package integrations_test

import (
	"database/sql/driver"
	"fmt"
	"os"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	sqlmock "gopkg.in/DATA-DOG/go-sqlmock.v1"
)

// the `prepare start-hub` tests are currently in master_only_integration_test
var _ = Describe("InitCluster", func() {
	BeforeEach(func() {
		go agent.Start()
	})
	AfterEach(func() {
		os.Remove(fmt.Sprintf("%s_upgrade", testWorkspaceDir))
	})
	It("executes gpinitsystem and returns a target cluster connector", func() {
		mockdb, mock := testhelper.CreateMockDB()
		testDriver := testhelper.TestDriver{DB: mockdb, DBName: "testdb", User: "testrole"}
		db := dbconn.NewDBConn(testDriver.DBName, testDriver.User, "fakehost", -1 /* not used */)
		db.Driver = testDriver

		mock.ExpectQuery("SELECT version()").WillReturnRows(getFakeVersionRow())
		checkpointRow := sqlmock.NewRows([]string{"string"}).AddRow(driver.Value("8"))
		encodingRow := sqlmock.NewRows([]string{"string"}).AddRow(driver.Value("UNICODE"))
		mock.ExpectQuery("SELECT .*checkpoint.*").WillReturnRows(checkpointRow)
		mock.ExpectQuery("SELECT .*server.*").WillReturnRows(encodingRow)
		mock.ExpectQuery("SELECT (.*)").WillReturnRows(getFakeConfigRows())

		targetConn, err := hub.InitCluster(db)
		Expect(err).ToNot(HaveOccurred())

		Expect(targetConn.Host).To(Equal("localhost"))
		Expect(targetConn.Port).To(Equal(15433))

		Expect(testExecutor.NumExecutions).To(Equal(1))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("gpinitsystem"))
	})
})

// Construct sqlmock in-memory rows that are structured properly
func getFakeVersionRow() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"versionstring"}).
		AddRow([]driver.Value{"PostgreSQL 8.4 (Greenplum Database 6.0.0)"}...)
}

func getFakeConfigRows() *sqlmock.Rows {
	header := []string{"dbid", "contentid", "port", "hostname", "datadir"}
	fakeConfigRow := []driver.Value{1, -1, 15432, "mdw", "/tmp/gpupgrade/seg-1_upgrade"}
	fakeConfigRow2 := []driver.Value{2, 0, 25432, "sdw1", "/tmp/gpupgrade/seg0_upgrade"}
	rows := sqlmock.NewRows(header)
	heapfakeResult := rows.AddRow(fakeConfigRow...).AddRow(fakeConfigRow2...)
	return heapfakeResult
}
