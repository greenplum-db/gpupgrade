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
	/* This is demonstrating the limited implementation of init-cluster.
	    Assuming the user has already set up their new cluster, they should `init-cluster`
		with the port at which they stood it up, so the upgrade tool can create new_cluster_config

		In the future, the upgrade tool might take responsibility for starting its own cluster,
		in which case it won't need the port, but would still generate new_cluster_config
	*/
	BeforeEach(func() {
		go agent.Start()
	})
	AfterEach(func() {
		os.Remove(fmt.Sprintf("%s_upgrade", testWorkspaceDir))
	})
	It("can save the database configuration json under the name 'new cluster'", func() {
		port := os.Getenv("PGPORT")
		Expect(port).ToNot(BeEmpty())

		mockdb, mock := testhelper.CreateMockDB()
		testDriver := testhelper.TestDriver{DB: mockdb, DBName: "testdb", User: "testrole"}
		db := dbconn.NewDBConnFromEnvironment("testdb")
		db.Driver = testDriver

		mock.ExpectQuery("SELECT version()").WillReturnRows(getFakeVersionRow())
		checkpointRow := sqlmock.NewRows([]string{"string"}).AddRow(driver.Value("8"))
		encodingRow := sqlmock.NewRows([]string{"string"}).AddRow(driver.Value("UNICODE"))
		mock.ExpectQuery("SELECT .*checkpoint.*").WillReturnRows(checkpointRow)
		mock.ExpectQuery("SELECT .*server.*").WillReturnRows(encodingRow)
		mock.ExpectQuery("SELECT (.*)").WillReturnRows(getFakeConfigRows())

		err := hub.InitCluster(db)
		Expect(err).ToNot(HaveOccurred())
		Expect(cm.WasReset(upgradestatus.INIT_CLUSTER)).To(BeTrue())
		Expect(cm.IsInProgress(upgradestatus.INIT_CLUSTER)).To(BeTrue())

		target := &utils.Cluster{ConfigPath: filepath.Join(testStateDir, utils.TARGET_CONFIG_FILENAME)}
		err = target.Load()
		Expect(err).ToNot(HaveOccurred())

		Expect(len(target.Segments)).To(BeNumerically(">", 1))
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
