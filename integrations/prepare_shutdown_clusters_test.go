package integrations_test

import (
	"errors"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("prepare shutdown-clusters", func() {
	var (
		mockAgent       *testutils.MockAgentServer
		testExecutorOld *testhelper.TestExecutor
		testExecutorNew *testhelper.TestExecutor
	)

	BeforeEach(func() {
		mockAgent, hubToAgentPort = testutils.NewMockAgentServer()

		testExecutorOld = &testhelper.TestExecutor{}
		testExecutorNew = &testhelper.TestExecutor{}
		cp.OldCluster.Executor = testExecutorOld
		cp.NewCluster.Executor = testExecutorNew
	})

	AfterEach(func() {
		mockAgent.Stop()
	})

	It("updates status PENDING, RUNNING then COMPLETE if successful", func() {
		mockAgent.StatusConversionResponse = &pb.CheckConversionStatusReply{
			Statuses: []string{},
		}

		Expect(cm.IsPending(upgradestatus.SHUTDOWN_CLUSTERS)).To(BeTrue())

		prepareShutdownClustersSession := runCommand("prepare", "shutdown-clusters", "--old-bindir", cp.OldBinDir, "--new-bindir", cp.NewBinDir)
		Eventually(prepareShutdownClustersSession).Should(Exit(0))

		Expect(testExecutorOld.NumExecutions).To(Equal(2))
		Expect(testExecutorOld.LocalCommands[0]).To(ContainSubstring("pgrep"))
		Expect(testExecutorOld.LocalCommands[1]).To(ContainSubstring(cp.OldBinDir + "/gpstop -a"))

		Expect(testExecutorNew.NumExecutions).To(Equal(2))
		Expect(testExecutorNew.LocalCommands[0]).To(ContainSubstring("pgrep"))
		Expect(testExecutorNew.LocalCommands[1]).To(ContainSubstring(cp.NewBinDir + "/gpstop -a"))

		Expect(cm.IsComplete(upgradestatus.SHUTDOWN_CLUSTERS)).To(BeTrue())
	})

	It("updates status to FAILED if it fails to run", func() {
		mockAgent.StatusConversionResponse = &pb.CheckConversionStatusReply{
			Statuses: []string{},
		}

		Expect(cm.IsPending(upgradestatus.SHUTDOWN_CLUSTERS)).To(BeTrue())

		testExecutorOld.ErrorOnExecNum = 2
		testExecutorNew.ErrorOnExecNum = 2
		testExecutorOld.LocalError = errors.New("stop failed")
		testExecutorNew.LocalError = errors.New("stop failed")

		prepareShutdownClustersSession := runCommand("prepare", "shutdown-clusters", "--old-bindir", cp.OldBinDir, "--new-bindir", cp.NewBinDir)
		Eventually(prepareShutdownClustersSession).Should(Exit(0))

		Expect(testExecutorOld.NumExecutions).To(Equal(2))
		Expect(testExecutorOld.LocalCommands[0]).To(ContainSubstring("pgrep"))
		Expect(testExecutorOld.LocalCommands[1]).To(ContainSubstring(cp.OldBinDir + "/gpstop -a"))
		Expect(testExecutorOld.NumExecutions).To(Equal(2))
		Expect(testExecutorNew.LocalCommands[0]).To(ContainSubstring("pgrep"))
		Expect(testExecutorNew.LocalCommands[1]).To(ContainSubstring(cp.NewBinDir + "/gpstop -a"))
		Expect(cm.IsFailed(upgradestatus.SHUTDOWN_CLUSTERS)).To(BeTrue())
	})
})
