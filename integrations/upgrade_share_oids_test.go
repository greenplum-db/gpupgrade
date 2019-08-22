package integrations_test

import (
	"errors"
	"os"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("upgrade share oids", func() {
	BeforeEach(func() {
		go agent.Start()
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}
		utils.System.Rename = func(oldpath, newpath string) error {
			return nil
		}
		utils.System.RemoveAll = func(name string) error {
			return nil
		}
	})

	It("updates status PENDING to RUNNING then to COMPLETE if successful", func() {
		Expect(cm.IsPending(upgradestatus.SHARE_OIDS)).To(BeTrue())

		upgradeShareOidsSession := runCommand("upgrade", "share-oids")
		Eventually(upgradeShareOidsSession).Should(Exit(0))

		Eventually(func() bool { return cm.IsComplete(upgradestatus.SHARE_OIDS) }).Should(BeTrue())
		Expect(testExecutor.ClusterCommands[0][0]).To(ContainElement("rsync"))
		Expect(len(agentExecutor.LocalCommands)).ToNot(Equal(0))
	})

	It("updates status to FAILED if it fails to run", func() {
		Expect(cm.IsPending(upgradestatus.SHARE_OIDS)).To(BeTrue())
		testExecutor.ClusterOutput = &cluster.RemoteOutput{
			NumErrors: 1,
			Errors: map[int]error{
				0: errors.New("fake test error, share oid failed to send files"),
			},
		}

		upgradeShareOidsSession := runCommand("upgrade", "share-oids")
		Eventually(upgradeShareOidsSession).Should(Exit(0))

		Eventually(func() bool { return cm.IsFailed(upgradestatus.SHARE_OIDS) }).Should(BeTrue())
		Expect(testExecutor.ClusterCommands[0][0]).To(ContainElement("rsync"))
	})
})
