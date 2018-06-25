package services_test

import (
	"database/sql/driver"

	"github.com/greenplum-db/gpupgrade/hub/services"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/utils"
	"gopkg.in/DATA-DOG/go-sqlmock.v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("hub", func() {
	var (
		dbConnector *dbconn.DBConn
		mock        sqlmock.Sqlmock
	)

	BeforeEach(func() {
		dbConnector, mock = testhelper.CreateAndConnectMockDB(1)
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		dbConnector.Close()
	})

	Describe("GetCountsForDb", func() {
		It("returns count for AO and HEAP tables", func() {
			fakeResults := sqlmock.NewRows([]string{"count"}).
				AddRow([]driver.Value{int32(2)}...)
			mock.ExpectQuery(".*c.relstorage IN.*").
				WillReturnRows(fakeResults)

			fakeResults = sqlmock.NewRows([]string{"count"}).
				AddRow([]driver.Value{int32(3)}...)
			mock.ExpectQuery(".*c.relstorage NOT IN.*").
				WillReturnRows(fakeResults)

			aocount, heapcount, err := services.GetCountsForDb(dbConnector)
			Expect(err).ToNot(HaveOccurred())
			Expect(aocount).To(Equal(int32(2)))
			Expect(heapcount).To(Equal(int32(3)))
		})
	})
})
