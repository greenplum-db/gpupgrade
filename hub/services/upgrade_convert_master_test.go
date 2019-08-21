package services_test

import (
	"errors"
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ConvertMasterHub", func() {
	var (
		testExecutor *testhelper.TestExecutor
		upgradeDir   string
	)

	BeforeEach(func() {
		testExecutor = &testhelper.TestExecutor{}
		source.Executor = testExecutor

		upgradeDir = utils.MasterPGUpgradeDirectory(dir)
	})

	It("returns with no error when convert master runs successfully", func() {
		err := hub.ConvertMaster()
		Expect(err).ToNot(HaveOccurred())

		Expect(testExecutor.LocalCommands[0]).To(Equal("source /target/greenplum_path.sh; " +
			fmt.Sprintf("cd %s; unset PGHOST; unset PGPORT; /target/bindir/pg_upgrade ", upgradeDir) +
			fmt.Sprintf("--old-bindir=/source/bindir --old-datadir=%s/seg-1 --old-port=15432 ", dir) +
			fmt.Sprintf("--new-bindir=/target/bindir --new-datadir=%s/seg-1 --new-port=15432 ", dir) +
			"--mode=dispatcher"))
	})

	It("returns an error when convert master fails", func() {
		testExecutor.LocalError = errors.New("upgrade failed")

		err := hub.ConvertMaster()
		Expect(err).To(HaveOccurred())
	})

	It("returns an error if the upgrade directory cannot be created", func() {
		testExecutor.LocalError = errors.New("failed to create directory")

		err := hub.ConvertMaster()
		Expect(err).To(HaveOccurred())
	})
})
