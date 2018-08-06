package services_test

import (
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gpupgrade/testutils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// XXX This, and the implementation of StartAgents(), are pretty much
// copy-pasted from hub_check_seginstall_test.go and VerifyAgentsInstalled().
// Consolidate.
var _ = Describe("StartAgents", func() {
	It("shells out to cluster and runs gpupgrade_agent", func() {
		source, _ := testutils.CreateMultinodeSampleClusterPair("/tmp")
		testExecutor := &testhelper.TestExecutor{}
		testExecutor.ClusterOutput = &cluster.RemoteOutput{}
		source.Executor = testExecutor

		err := services.StartAgents(source)
		Expect(err).NotTo(HaveOccurred())

		Expect(testExecutor.NumExecutions).To(Equal(1))

		startAgentsCmd := fmt.Sprintf("%s/gpupgrade_agent --daemonize", source.BinDir)
		clusterCommands := testExecutor.ClusterCommands[0]
		for _, command := range clusterCommands {
			Expect(command).To(ContainElement(startAgentsCmd))
		}
	})

	It("returns an error if the source cluster is not initialized", func() {
		source := &utils.Cluster{Cluster: &cluster.Cluster{}}
		err := services.StartAgents(source)
		Expect(err).To(HaveOccurred())
	})

	It("returns an error if any agents report an error", func() {
		source, _ := testutils.CreateMultinodeSampleClusterPair("/tmp")
		testExecutor := &testhelper.TestExecutor{}
		testExecutor.ClusterOutput = &cluster.RemoteOutput{
			NumErrors: 1,
			Errors:    map[int]error{0: fmt.Errorf("error")},
			Stdouts:   map[int]string{0: ""},
			Stderrs:   map[int]string{0: ""},
			CmdStrs:   map[int]string{0: ""},
		}
		source.Cluster.Executor = testExecutor

		err := services.StartAgents(source)
		Expect(err).To(HaveOccurred())
	})
})
