package services_test

import (
	"errors"
	"fmt"

	"github.com/blang/semver"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ConvertMasterHub", func() {
	var (
		testExecutor *testhelper.TestExecutor
	)

	BeforeEach(func() {
		testExecutor = &testhelper.TestExecutor{}
		source.Executor = testExecutor
	})

	It("returns with no error when convert master runs successfully", func() {
		err := hub.ConvertMaster()
		Expect(err).ToNot(HaveOccurred())

		Expect(testExecutor.LocalCommands[0]).To(Equal(fmt.Sprintf("unset PGHOST; unset PGPORT; /target/bindir/pg_upgrade ") +
			fmt.Sprintf("--old-bindir=/source/bindir --old-datadir=%s/seg-1 --old-port=15432 ", dir) +
			fmt.Sprintf("--new-bindir=/target/bindir --new-datadir=%s/seg-1 --new-port=15432 ", dir) +
			"--mode=dispatcher"))
	})

	It("uses the correct pg_upgrade options for older DBs", func() {
		target.Version = dbconn.GPDBVersion{
			VersionString: "5.0.0",
			SemVer:        semver.MustParse("5.0.0"),
		}

		err := hub.ConvertMaster()
		Expect(err).ToNot(HaveOccurred())

		Expect(testExecutor.LocalCommands[0]).To(Equal(fmt.Sprintf("unset PGHOST; unset PGPORT; /target/bindir/pg_upgrade ") +
			fmt.Sprintf("--old-bindir=/source/bindir --old-datadir=%s/seg-1 --old-port=15432 ", dir) +
			fmt.Sprintf("--new-bindir=/target/bindir --new-datadir=%s/seg-1 --new-port=15432 ", dir) +
			"--dispatcher-mode"))
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
