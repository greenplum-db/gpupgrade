package services_test

import (
	"github.com/greenplum-db/gpupgrade/agent/services"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CommandListener", func() {
	BeforeEach(func() {
		testhelper.SetupTestLogger()
	})

	AfterEach(func() {
		//any mocking of utils.System function pointers should be reset by calling InitializeSystemFunctions
		utils.System = utils.InitializeSystemFunctions()
	})

	It("returns an empty reply", func() {
		testExecutor := &testhelper.TestExecutor{}
		agent := services.NewAgentServer(testExecutor, services.AgentConfig{})

		_, err := agent.PingAgents(nil, &pb.PingAgentsRequest{})
		Expect(err).To(BeNil())
	})
})
