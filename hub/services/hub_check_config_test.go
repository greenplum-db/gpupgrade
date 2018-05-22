package services_test

import (
	"database/sql/driver"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/greenplum-db/gpupgrade/hub/services"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/operating"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"gopkg.in/DATA-DOG/go-sqlmock.v1"
)

var _ = Describe("Hub check config", func() {
	var (
		dbConnector *dbconn.DBConn
		mock        sqlmock.Sqlmock
		dir         string
		err         error
		oldBinDir   string
		queryResult = `{"SegConfig":[{"address":"mdw","content":-1,"datadir":"/data/master/gpseg-1","dbid":1,"hostname":"mdw","mode":"s","status":"u","port":15432,"preferred_role":"p","role":"p"},` +
			`{"address":"sdw1","content":0,"datadir":"/data/primary/gpseg-0","dbid":2,"hostname":"sdw1","mode":"s","status":"u","port":25432,"preferred_role":"p","role":"p"}],"BinDir":"/tmp"}`
	)

	BeforeEach(func() {
		oldBinDir = "/tmp"
		dbConnector, mock = testhelper.CreateAndConnectMockDB(1)
		dir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
		operating.System = operating.InitializeSystemFunctions()
	})

	AfterEach(func() {
		operating.System = operating.InitializeSystemFunctions()
	})

	It("successfully writes config for GPDB 6", func() {
		testhelper.SetDBVersion(dbConnector, "6.0.0")

		configQuery := services.CONFIGQUERY6

		mock.ExpectQuery(configQuery).WillReturnRows(getFakeConfigRows())

		fakeConfigFile := gbytes.NewBuffer()

		operating.System.OpenFileWrite = func(name string, flag int, perm os.FileMode) (io.WriteCloser, error) {
			return fakeConfigFile, nil
		}

		err := services.SaveOldClusterConfig(dbConnector, dir, oldBinDir)
		Expect(err).ToNot(HaveOccurred())

		Expect(string(fakeConfigFile.Contents())).To(ContainSubstring(queryResult))
	})

	// The database is running, master-host is provided, and connection is successful
	// writes the resulting rows according to however the provided writer does it
	It("successfully writes config for GPDB 4 and 5", func() {
		configQuery := services.CONFIGQUERY5

		mock.ExpectQuery(configQuery).WillReturnRows(getFakeConfigRows())

		fakeConfigFile := gbytes.NewBuffer()

		operating.System.OpenFileWrite = func(name string, flag int, perm os.FileMode) (io.WriteCloser, error) {
			return fakeConfigFile, nil
		}

		err := services.SaveOldClusterConfig(dbConnector, dir, oldBinDir)
		Expect(err).ToNot(HaveOccurred())

		Expect(string(fakeConfigFile.Contents())).To(ContainSubstring(queryResult))
	})

	It("fails to get config file handle", func() {
		operating.System.OpenFileWrite = func(name string, flag int, perm os.FileMode) (io.WriteCloser, error) {
			return nil, errors.New("failed to write config file")
		}

		err := services.SaveOldClusterConfig(dbConnector, dir, oldBinDir)
		Expect(err).To(HaveOccurred())
	})

	It("db.Select query for cluster config fails", func() {
		configQuery := services.CONFIGQUERY5
		mock.ExpectQuery(configQuery).WillReturnError(errors.New("fail config query"))

		operating.System.OpenFileWrite = func(name string, flag int, perm os.FileMode) (io.WriteCloser, error) {
			return gbytes.NewBuffer(), nil
		}

		err := services.SaveOldClusterConfig(dbConnector, dir, oldBinDir)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("Unable to execute query " + configQuery + ". Err: fail config query"))
	})

	It("fails to save query result to file", func() {
		// To force a write error in SaveQueryResultToJSON we create the file with read only mode
		fileHandle, err := operating.System.OpenFileWrite(filepath.Join(dir, "readOnlyFile"), os.O_RDONLY|os.O_CREATE, 0400)
		Expect(err).ToNot(HaveOccurred())

		err = services.SaveQueryResultToJSON(nil, fileHandle)
		Expect(err).To(HaveOccurred())
	})
})

// Construct sqlmock in-memory rows that are structured properly
func getFakeConfigRows() *sqlmock.Rows {
	header := []string{"address", "content", "datadir", "dbid", "hostname", "mode", "status", "port", "preferred_role", "role"}
	fakeConfigRow := []driver.Value{"mdw", -1, "/data/master/gpseg-1", 1, "mdw", "s", "u", 15432, "p", "p"}
	fakeConfigRow2 := []driver.Value{"sdw1", 0, "/data/primary/gpseg-0", 2, "sdw1", "s", "u", 25432, "p", "p"}
	rows := sqlmock.NewRows(header)
	heapfakeResult := rows.AddRow(fakeConfigRow...).AddRow(fakeConfigRow2...)
	return heapfakeResult
}
