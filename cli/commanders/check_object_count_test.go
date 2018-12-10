package commanders_test

import (
	"errors"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/mock_idl"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("object count tests", func() {

	var (
		client *mock_idl.MockCliToHubClient
		ctrl   *gomock.Controller
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		client = mock_idl.NewMockCliToHubClient(ctrl)
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		defer ctrl.Finish()
	})
	Describe("Execute", func() {
		It("prints out that check object count request was processed", func() {
			//testLogger, testStdout, testStderr, testLogfile := testutils.SetupTestLogger()
			testStdout, _, _ := testhelper.SetupTestLogger()

			fakeCountArray := []*idl.CountPerDb{}
			fakeCountTemplate1 := &idl.CountPerDb{DbName: "template1", AoCount: 1, HeapCount: 2}
			fakeCountArray = append(fakeCountArray, fakeCountTemplate1)
			fakeCountPostgres := &idl.CountPerDb{DbName: "postgres", AoCount: 2, HeapCount: 3}
			fakeCountArray = append(fakeCountArray, fakeCountPostgres)
			fakeCheckObjectCountReply := &idl.CheckObjectCountReply{ListOfCounts: fakeCountArray}

			client.EXPECT().CheckObjectCount(
				gomock.Any(),
				&idl.CheckObjectCountRequest{},
			).Return(fakeCheckObjectCountReply, nil)

			request := commanders.NewObjectCountChecker(client)
			err := request.Execute()
			Expect(err).To(BeNil())
			Eventually(testStdout).Should(gbytes.Say("Checking object counts in database: template1"))
			Eventually(testStdout).Should(gbytes.Say("Number of AO objects - 1"))
			Eventually(testStdout).Should(gbytes.Say("Number of heap objects - 2"))
			Eventually(testStdout).Should(gbytes.Say("Checking object counts in database: postgres"))
			Eventually(testStdout).Should(gbytes.Say("Number of AO objects - 2"))
			Eventually(testStdout).Should(gbytes.Say("Number of heap objects - 3"))
			Eventually(testStdout).Should(gbytes.Say("Check object count request is processed."))
		})

		It("prints out an error when connection cannot be established to the hub", func() {
			_, testStderr, _ := testhelper.SetupTestLogger()
			client.EXPECT().CheckObjectCount(
				gomock.Any(),
				&idl.CheckObjectCountRequest{},
			).Return(nil, errors.New("Force failure connection"))

			request := commanders.NewObjectCountChecker(client)
			err := request.Execute()
			Expect(err).ToNot(BeNil())
			Eventually(testStderr).Should(gbytes.Say("ERROR - gRPC call to hub failed"))

		})
	})
})
