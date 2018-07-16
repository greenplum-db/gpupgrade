package integrations_test

import (
	"fmt"

	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

// needs the cli and the hub
var _ = Describe("check config", func() {
	It("happy: the database configuration is saved to a specified location", func() {
		session := runCommand("check", "config", "--master-host", "localhost", "--old-bindir", "/old/bin/dir")
		if session.ExitCode() != 0 {
			fmt.Println("make sure greenplum is running")
		}
		Expect(session).To(Exit(0))

		cp := &utils.ClusterPair{}
		err := cp.ReadOldConfig(testStateDir)
		testutils.Check("cannot read config", err)

		Expect(len(cp.OldCluster.Segments)).To(BeNumerically(">", 1))
	})
})
