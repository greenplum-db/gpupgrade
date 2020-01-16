package hub_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	sqlmock "gopkg.in/DATA-DOG/go-sqlmock.v1"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/mock_agent"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	ctrl        *gomock.Controller
	dbConnector *dbconn.DBConn
	mock        sqlmock.Sqlmock
	mockAgent   *mock_agent.MockAgentServer
	dialer      hub.Dialer
	client      *mock_idl.MockAgentClient
	cm          *testutils.MockChecklistManager
	port        int
	dir         string
	hubConf     *hub.Config
	source      *utils.Cluster
	target      *utils.Cluster
	testHub     *hub.Hub
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
	mockAgent, dialer, port = mock_agent.NewMockAgentServer()
	client = mock_idl.NewMockAgentClient(ctrl)
	conf := &hub.Config{source, target, 0, port}
	testHub = hub.New(conf, dialer, dir, cm)
})

var _ = AfterEach(func() {
	dbConnector.Close()
	utils.System = utils.InitializeSystemFunctions()
	ctrl.Finish()
	os.RemoveAll(dir)
})

// Enable exectest.NewCommand mocking.
func TestMain(m *testing.M) {

	os.Exit(exectest.Run(m))
}
