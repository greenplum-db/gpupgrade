package integrations_test

import (
	"database/sql/driver"
	"fmt"
	"os"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/testutils"
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
		mockdb, mock := testhelper.CreateMockDBConn()
		testhelper.ExpectVersionQuery(mock, "6.0.0")

		checkpointRow := sqlmock.NewRows([]string{"string"}).AddRow(driver.Value("8"))
		encodingRow := sqlmock.NewRows([]string{"string"}).AddRow(driver.Value("UNICODE"))
		mock.ExpectQuery("SELECT .*checkpoint.*").WillReturnRows(checkpointRow)
		mock.ExpectQuery("SELECT .*server.*").WillReturnRows(encodingRow)
		mock.ExpectQuery("SELECT (.*)").WillReturnRows(testutils.MockSegmentConfiguration())

		targetConn, err := hub.InitCluster(mockdb)
		Expect(err).ToNot(HaveOccurred())

		Expect(targetConn.Host).To(Equal("localhost"))
		Expect(targetConn.Port).To(Equal(15433))

		Expect(testExecutor.NumExecutions).To(Equal(1))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("gpinitsystem"))
	})
})
