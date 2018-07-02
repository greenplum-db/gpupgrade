package services_test

import (
	"context"

	"github.com/greenplum-db/gpupgrade/agent/services"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/onsi/gomega/gbytes"
	"github.com/pkg/errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CommandListener", func() {
	var (
		testLogFile  *gbytes.Buffer
		agent        *services.AgentServer
		testExecutor *testhelper.TestExecutor
	)

	BeforeEach(func() {
		_, _, testLogFile = testhelper.SetupTestLogger()

		testExecutor = &testhelper.TestExecutor{}
		agent = services.NewAgentServer(testExecutor, services.AgentConfig{})
	})

	AfterEach(func() {
		//any mocking of utils.System function pointers should be reset by calling InitializeSystemFunctions
		utils.System = utils.InitializeSystemFunctions()
	})

	It("returns the shell command output", func() {
		testExecutor.LocalOutput = "shell command output"

		resp, err := agent.CheckUpgradeStatus(context.TODO(), nil)
		Expect(err).ToNot(HaveOccurred())

		Expect(resp.ProcessList).To(Equal("shell command output"))
	})

	It("returns only err if anything is reported as an error", func() {
		testExecutor.LocalError = errors.New("couldn't find bash")

		resp, err := agent.CheckUpgradeStatus(context.TODO(), nil)
		Expect(err).To(HaveOccurred())

		Expect(resp).To(BeNil())
		Expect(testLogFile.Contents()).To(ContainSubstring("couldn't find bash"))
	})
})
