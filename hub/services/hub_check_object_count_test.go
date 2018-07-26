package services_test

import (
	"database/sql/driver"

	"github.com/greenplum-db/gpupgrade/hub/services"

	"gopkg.in/DATA-DOG/go-sqlmock.v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("hub", func() {
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
