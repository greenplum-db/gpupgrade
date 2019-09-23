package hub_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/mock/gomock"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"

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
	port        int
	dir         string
	hubConf     *hub.Config
	source      *utils.Cluster
	target      *utils.Cluster
	testHub     *hub.Hub
	useLinkMode bool
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

	var err error
	dir, err = ioutil.TempDir("", "")
	Expect(err).ToNot(HaveOccurred())

	source, target = testutils.CreateMultinodeSampleClusterPair(dir)
	mockAgent, dialer, port = mock_agent.NewMockAgentServer()
	client = mock_idl.NewMockAgentClient(ctrl)
	useLinkMode = false
	conf := &hub.Config{source, target, 0, port, useLinkMode}
	testHub = hub.New(conf, dialer, dir)
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
