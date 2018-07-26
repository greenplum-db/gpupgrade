package services_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gpupgrade/hub/services"
	mockpb "github.com/greenplum-db/gpupgrade/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
	"google.golang.org/grpc"
	sqlmock "gopkg.in/DATA-DOG/go-sqlmock.v1"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	ctrl        *gomock.Controller
	dbConnector *dbconn.DBConn
	mock        sqlmock.Sqlmock
	mockAgent   *testutils.MockAgentServer
	client      *mockpb.MockAgentClient
	cm          *testutils.MockChecklistManager
	port        int
	dir         string
	hubConf     *services.HubConfig
	source      *utils.Cluster
	target      *utils.Cluster
	hub         *services.Hub
)

func TestCommands(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Hub Services Suite")
}

var _ = BeforeSuite(func() {
	testhelper.SetupTestLogger()
	utils.System = utils.InitializeSystemFunctions()
})

var _ = BeforeEach(func() {
	ctrl = gomock.NewController(GinkgoT())
	dbConnector, mock = testhelper.CreateAndConnectMockDB(1)
	cm = testutils.NewMockChecklistManager()

	var err error
	dir, err = ioutil.TempDir("", "")
	Expect(err).ToNot(HaveOccurred())

	source, target = testutils.CreateMultinodeSampleClusterPair(dir)
	mockAgent, port = testutils.NewMockAgentServer()
	client = mockpb.NewMockAgentClient(ctrl)
	hubConf = &services.HubConfig{
		HubToAgentPort: port,
		StateDir:       dir,
	}
	hub = services.NewHub(source, target, grpc.DialContext, hubConf, cm)
})

var _ = AfterEach(func() {
	dbConnector.Close()
	utils.System = utils.InitializeSystemFunctions()
	ctrl.Finish()
	os.RemoveAll(dir)
})
