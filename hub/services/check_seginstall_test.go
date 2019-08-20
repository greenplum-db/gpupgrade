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

var _ = Describe("VerifyAgentsInstalled", func() {
	It("shells out to cluster and verifies gpupgrade_agent is installed on master and hosts", func() {
		source, target := testutils.CreateMultinodeSampleClusterPair("/tmp")
		testExecutor := &testhelper.TestExecutor{}
		testExecutor.ClusterOutput = &cluster.RemoteOutput{}
		source.Cluster.Executor = testExecutor

		err := services.VerifyAgentsInstalled(source, target)
		Expect(err).NotTo(HaveOccurred())

		Expect(testExecutor.NumExecutions).To(Equal(1))

		lsCmd := fmt.Sprintf("ls %s/gpupgrade_agent", target.BinDir)
		clusterCommands := testExecutor.ClusterCommands[0]
		for _, command := range clusterCommands {
			Expect(command).To(ContainElement(lsCmd))
		}
	})

	It("returns an error if the source cluster is not initialized", func() {
		source := &utils.Cluster{Cluster: &cluster.Cluster{}}
		target := &utils.Cluster{Cluster: &cluster.Cluster{}}
		err := services.VerifyAgentsInstalled(source, target)
		Expect(err).To(HaveOccurred())
	})

	It("returns an error if any agents report an error", func() {
		source, target := testutils.CreateMultinodeSampleClusterPair("/tmp")
		testExecutor := &testhelper.TestExecutor{}
		testExecutor.ClusterOutput = &cluster.RemoteOutput{
			NumErrors: 1,
			Errors:    map[int]error{0: fmt.Errorf("error")},
			Stdouts:   map[int]string{0: ""},
			Stderrs:   map[int]string{0: ""},
			CmdStrs:   map[int]string{0: ""},
		}
		source.Cluster.Executor = testExecutor

		err := services.VerifyAgentsInstalled(source, target)
		Expect(err).To(HaveOccurred())
	})
})
