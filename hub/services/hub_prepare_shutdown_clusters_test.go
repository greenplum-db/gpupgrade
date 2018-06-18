package services_test

import (
	"errors"
	"io/ioutil"
	"os"

	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/testutils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("PrepareShutdownClusters", func() {
	var (
		conf               *services.HubConfig
		testLog            *gbytes.Buffer
		stubRemoteExecutor *testutils.StubRemoteExecutor
		clusterPair        *services.ClusterPair
		cm                 *testutils.MockChecklistManager
	)
	BeforeEach(func() {
		_, _, testLog = testhelper.SetupTestLogger()
		utils.System.RemoveAll = func(s string) error { return nil }
		utils.System.MkdirAll = func(s string, perm os.FileMode) error { return nil }

		dir, err := ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())
		conf = &services.HubConfig{
			StateDir: dir,
		}
		stubRemoteExecutor = testutils.NewStubRemoteExecutor()
		clusterPair = testutils.CreateSampleClusterPair()
		clusterPair.OldCluster.Executor = &testhelper.TestExecutor{}
		cm = testutils.NewMockChecklistManager()
	})

	AfterEach(func() {
		utils.InitializeSystemFunctions()
	})

	It("isPostmasterRunning() succeeds", func() {
		testExecutor := &testhelper.TestExecutor{}

		cluster := testutils.CreateSampleCluster(-1, 25437, "hostone",
			"/master/datadir")
		cluster.Executor = testExecutor
		postmasterRunning := services.IsPostmasterRunning(cluster)
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("pgrep"))
		Expect(postmasterRunning).To(BeTrue())
	})

	It("isPostmasterRunning() fails", func() {
		testExecutor := &testhelper.TestExecutor{}
		testExecutor.LocalError = errors.New("some error message")

		cluster := testutils.CreateSampleCluster(-1, 25437, "hostone",
			"/master/datadir")
		cluster.Executor = testExecutor
		postmasterRunning := services.IsPostmasterRunning(cluster)
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("pgrep"))
		Expect(postmasterRunning).To(BeFalse())
	})

	It("stopCluster() succeesfully shuts down cluster", func() {
		testExecutor := &testhelper.TestExecutor{}

		cluster := testutils.CreateSampleCluster(-1, 25437, "hostone",
			"/master/datadir")
		cluster.Executor = testExecutor

		err := services.StopCluster(cluster, "/fake/bindir")

		Expect(testExecutor.NumExecutions).To(Equal(2))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("pgrep"))
		Expect(testExecutor.LocalCommands[1]).To(ContainSubstring("gpstop"))
		Expect(err).ToNot(HaveOccurred())
	})

	It("stopCluster() detects that cluster is already shutdown", func() {
		testExecutor := &testhelper.TestExecutor{}
		testExecutor.LocalError = errors.New("some error message")

		cluster := testutils.CreateSampleCluster(-1, 25437, "hostone",
			"/master/datadir")
		cluster.Executor = testExecutor

		err := services.StopCluster(cluster, "/fake/bindir")

		Expect(testExecutor.NumExecutions).To(Equal(1))
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring("pgrep"))
		Expect(err).ToNot(HaveOccurred())
	})

})
