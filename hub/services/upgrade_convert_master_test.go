package services_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ConvertMasterHub", func() {
	var (
		actualCmdStr string
	)

	BeforeEach(func() {
		utils.System.RunCommandAsync = func(cmdStr string, logFile string) error {
			actualCmdStr = cmdStr
			return nil
		}
	})

	It("returns with no error when convert master runs successfully", func() {
		err := hub.ConvertMaster()
		Expect(err).ToNot(HaveOccurred())

		pgupgrade_dir := filepath.Join(dir, "pg_upgrade")
		Expect(actualCmdStr).To(Equal(fmt.Sprintf("unset PGHOST; unset PGPORT; cd %s && nohup /target/bindir/pg_upgrade ", pgupgrade_dir) +
			fmt.Sprintf("--old-bindir=/source/bindir --old-datadir=%s/seg-1 --old-port=15432 ", dir) +
			fmt.Sprintf("--new-bindir=/target/bindir --new-datadir=%s/seg-1 --new-port=15432 ", dir) +
			"--dispatcher-mode --progress"))
	})

	It("returns an error when convert master fails", func() {
		utils.System.RunCommandAsync = func(cmdStr string, logFile string) error {
			return errors.New("upgrade failed")
		}

		err := hub.ConvertMaster()
		Expect(err).To(HaveOccurred())
	})

	It("returns an error if the upgrade directory cannot be created", func() {
		utils.System.MkdirAll = func(path string, perm os.FileMode) error {
			return errors.New("failed to create directory")
		}

		err := hub.ConvertMaster()
		Expect(err).To(HaveOccurred())
	})
})
