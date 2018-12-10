package services_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gpupgrade/agent/services"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("UpgradeSegments", func() {
	var (
		agent        *services.AgentServer
		dir          string
		oidFile      string
		testExecutor *testhelper.TestExecutor
		dataDirPairs []*idl.DataDirPair
	)

	BeforeEach(func() {
		testhelper.SetupTestLogger()

		var err error
		dir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		testExecutor = &testhelper.TestExecutor{}
		agentConfig := services.AgentConfig{StateDir: dir}
		agent = services.NewAgentServer(testExecutor, agentConfig)

		upgradeDir := utils.PGUpgradeDirectory(dir)

		err = os.MkdirAll(upgradeDir, 0700)
		Expect(err).ToNot(HaveOccurred())

		oidFile = filepath.Join(upgradeDir, "pg_upgrade_dump_seg1_oids.sql")
		f, err := os.Create(oidFile)
		Expect(err).ToNot(HaveOccurred())
		f.Close()

		dataDirPairs = []*idl.DataDirPair{
			{OldDataDir: "old/datadir1", NewDataDir: "new/datadir1", Content: 0, OldPort: 1, NewPort: 11},
			{OldDataDir: "old/datadir2", NewDataDir: "new/datadir2", Content: 1, OldPort: 2, NewPort: 22},
		}
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
	})

	It("successfully runs pg_upgrade", func() {
		// We want to check what commands are passed to RunCommandAsync, so we have testExecutor record them for us
		utils.System.RunCommandAsync = func(cmdStr, logFile string) error {
			_, err := testExecutor.ExecuteLocalCommand(cmdStr)
			return err
		}

		err := agent.UpgradeSegments("/old/bin", "/new/bin", "6.0.0", dataDirPairs)
		Expect(err).ToNot(HaveOccurred())

		Expect(testExecutor.NumExecutions).To(Equal(2))

		upgradeDir0 := utils.SegmentPGUpgradeDirectory(dir, 0)
		upgradeDir1 := utils.SegmentPGUpgradeDirectory(dir, 1)

		Expect(testExecutor.LocalCommands).To(ContainElement(fmt.Sprintf("source /new/greenplum_path.sh; cd %s; unset PGHOST; unset PGPORT; /new/bin/pg_upgrade --old-bindir=/old/bin --old-datadir=old/datadir1 --old-port=1 --new-bindir=/new/bin --new-datadir=new/datadir1 --new-port=11 --mode=segment --progress", upgradeDir0)))
		Expect(testExecutor.LocalCommands).To(ContainElement(fmt.Sprintf("source /new/greenplum_path.sh; cd %s; unset PGHOST; unset PGPORT; /new/bin/pg_upgrade --old-bindir=/old/bin --old-datadir=old/datadir2 --old-port=2 --new-bindir=/new/bin --new-datadir=new/datadir2 --new-port=22 --mode=segment --progress", upgradeDir1)))
	})

	Context("for older Greenplum versions", func() {
		It("successfully runs pg_upgrade", func() {
			// We want to check what commands are passed to RunCommandAsync, so we have testExecutor record them for us
			utils.System.RunCommandAsync = func(cmdStr, logFile string) error {
				_, err := testExecutor.ExecuteLocalCommand(cmdStr)
				return err
			}

			err := agent.UpgradeSegments("/old/bin", "/new/bin", "5.3.0", dataDirPairs)
			Expect(err).ToNot(HaveOccurred())

			Expect(testExecutor.NumExecutions).To(Equal(4))

			upgradeDir0 := utils.SegmentPGUpgradeDirectory(dir, 0)
			upgradeDir1 := utils.SegmentPGUpgradeDirectory(dir, 1)

			Expect(testExecutor.LocalCommands).To(ContainElement(fmt.Sprintf("cp %s %s", oidFile, upgradeDir0)))
			Expect(testExecutor.LocalCommands).To(ContainElement(fmt.Sprintf("source /new/greenplum_path.sh; cd %s; unset PGHOST; unset PGPORT; /new/bin/pg_upgrade --old-bindir=/old/bin --old-datadir=old/datadir1 --old-port=1 --new-bindir=/new/bin --new-datadir=new/datadir1 --new-port=11 --progress", upgradeDir0)))
			Expect(testExecutor.LocalCommands).To(ContainElement(fmt.Sprintf("cp %s %s", oidFile, upgradeDir1)))
			Expect(testExecutor.LocalCommands).To(ContainElement(fmt.Sprintf("source /new/greenplum_path.sh; cd %s; unset PGHOST; unset PGPORT; /new/bin/pg_upgrade --old-bindir=/old/bin --old-datadir=old/datadir2 --old-port=2 --new-bindir=/new/bin --new-datadir=new/datadir2 --new-port=22 --progress", upgradeDir1)))
		})

		It("returns an an error if the oid files glob fails", func() {
			utils.System.FilePathGlob = func(pattern string) ([]string, error) {
				return []string{}, errors.New("failed to find files")
			}

			err := agent.UpgradeSegments("/old/bin", "/new/bin", "5.3.0", dataDirPairs)
			Expect(err).To(HaveOccurred())
		})

		It("returns an an error if no oid files are found", func() {
			err := os.Remove(oidFile)
			Expect(err).ToNot(HaveOccurred())

			err = agent.UpgradeSegments("/old/bin", "/new/bin", "5.3.0", dataDirPairs)
			Expect(err).To(HaveOccurred())
		})

		It("returns an error if the oid files fail to copy into the segment directory", func() {
			testExecutor.LocalError = errors.New("Failed to copy oid file into segment directory")

			err := agent.UpgradeSegments("/old/bin", "/new/bin", "5.3.0", dataDirPairs)
			Expect(err).To(HaveOccurred())
		})
	})

	It("returns an error if the target version is incomprehensible", func() {
		err := agent.UpgradeSegments("/old/bin", "/new/bin", "klf;adsfds", dataDirPairs)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to parse new cluster version"))
	})

	It("returns an error if the pg_upgrade/segmentDir cannot be made", func() {
		utils.System.MkdirAll = func(path string, perm os.FileMode) error {
			return errors.New("failed to create segment directory")
		}

		err := agent.UpgradeSegments("/old/bin", "/new/bin", "6.0.0", dataDirPairs)
		Expect(err).To(HaveOccurred())
	})

	It("returns an error if starting pg_upgrade fails", func() {
		cmdErr := errors.New("convert primary on agent failed")
		utils.System.RunCommandAsync = func(cmdStr, logFile string) error {
			return cmdErr
		}

		err := agent.UpgradeSegments("/old/bin", "/new/bin", "6.0.0", dataDirPairs)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(cmdErr.Error()))
	})
})
