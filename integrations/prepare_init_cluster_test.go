package integrations_test

import (
	"database/sql/driver"
	"fmt"
	"os"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	sqlmock "gopkg.in/DATA-DOG/go-sqlmock.v1"
)

// the `prepare start-hub` tests are currently in master_only_integration_test
var _ = Describe("prepare", func() {
	var (
		targetPath string
	)
	BeforeEach(func() {
		step := cm.GetStepWriter(upgradestatus.INIT_CLUSTER)
		step.ResetStateDir()
		step.MarkInProgress()
		go agent.Start()
		targetPath = filepath.Join(testStateDir, utils.TARGET_CONFIG_FILENAME)
	})
	AfterEach(func() {
		os.Remove(fmt.Sprintf("%s_upgrade", testWorkspaceDir))
	})
	It("can save the newly minted database configuration json to disk", func() {
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

		seedCluster, err := hub.InitCluster(db)
		Expect(err).ToNot(HaveOccurred())
		Expect(cm.WasReset(upgradestatus.INIT_CLUSTER)).To(BeTrue())
		Expect(cm.IsInProgress(upgradestatus.INIT_CLUSTER)).To(BeTrue())
		seedCluster.ConfigPath = targetPath

		mockdb, mock = testhelper.CreateMockDB()
		testDriver = testhelper.TestDriver{DB: mockdb, DBName: "testdb", User: "testrole"}
		db = dbconn.NewDBConn(testDriver.DBName, testDriver.User, "fakehost", -1 /* not used */)
		db.Driver = testDriver

		mock.ExpectQuery("SELECT version()").WillReturnRows(getFakeVersionRow())
		checkpointRow = sqlmock.NewRows([]string{"string"}).AddRow(driver.Value("8"))
		encodingRow = sqlmock.NewRows([]string{"string"}).AddRow(driver.Value("UNICODE"))
		mock.ExpectQuery("SELECT (.*)").WillReturnRows(getFakeConfigRows())

		err = seedCluster.RefreshConfig(db)
		Expect(err).ToNot(HaveOccurred())
		err = seedCluster.Commit()
		Expect(err).ToNot(HaveOccurred())

		target := &utils.Cluster{ConfigPath: targetPath}
		err = target.Load()
		Expect(err).ToNot(HaveOccurred())

		Expect(len(target.Segments)).To(BeNumerically(">", 1), "number of segements")
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
