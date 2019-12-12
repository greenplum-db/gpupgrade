package services_test

import (
	"os"
	"strings"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/agent/services"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CopyMasterDirectoryToSegmentDirectories", func() {
	var (
		agentRequest *idl.CopyMasterDirRequest
		testExecutor *testhelper.TestExecutor
		stattedFiles []string
		renamedFiles []string
		deletedDirs  []string
	)

	BeforeEach(func() {
		testhelper.SetupTestLogger()

		agentRequest = &idl.CopyMasterDirRequest{
			MasterDir: "/tmp/masterDataDir",
			Datadirs:  []string{"/tmp/dataDir0", "/tmp/dataDir1"},
		}

		testExecutor = &testhelper.TestExecutor{}

		stattedFiles = []string{}
		renamedFiles = []string{}
		deletedDirs = []string{}

		utils.System.Stat = func(name string) (os.FileInfo, error) {
			stattedFiles = append(stattedFiles, name)
			return nil, nil
		}

		utils.System.Rename = func(oldpath, newpath string) error {
			renamedFiles = append(renamedFiles, newpath)
			return nil
		}

		utils.System.RemoveAll = func(name string) error {
			deletedDirs = append(deletedDirs, name)
			return nil
		}
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
	})

	It("copies master directory to segment directories successfully", func() {
		agent := services.NewAgentServer(testExecutor, services.AgentConfig{})
		_, err := agent.CopyMasterDirectoryToSegmentDirectories(nil, agentRequest)
		Expect(err).To(BeNil())

		Expect(stattedFiles).To(Equal([]string{"/tmp/dataDir0", "/tmp/dataDir1"}))

		Expect(testExecutor.LocalCommands).To(Equal([]string{
			"cp -a /tmp/masterDataDir /tmp/dataDir0",
			"cp -a /tmp/masterDataDir /tmp/dataDir1"}))

		Expect(renamedFiles).To(Equal([]string{
			"/tmp/dataDir0.old",
			"/tmp/dataDir0/internal.auto.conf",
			"/tmp/dataDir0/postgresql.conf",
			"/tmp/dataDir0/pg_hba.conf",
			"/tmp/dataDir0/postmaster.opts",
			"/tmp/dataDir1.old",
			"/tmp/dataDir1/internal.auto.conf",
			"/tmp/dataDir1/postgresql.conf",
			"/tmp/dataDir1/pg_hba.conf",
			"/tmp/dataDir1/postmaster.opts",
		}))

		Expect(deletedDirs).To(Equal([]string{
			"/tmp/dataDir0/gp_dbid",
			"/tmp/dataDir0/gpssh.conf",
			"/tmp/dataDir0/gpperfmon",
			"/tmp/dataDir1/gp_dbid",
			"/tmp/dataDir1/gpssh.conf",
			"/tmp/dataDir1/gpperfmon",
			"/tmp/masterDataDir"}))
	})

	It("errors when failing when segment data directory does not exist", func() {
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		agent := services.NewAgentServer(testExecutor, services.AgentConfig{})
		_, err := agent.CopyMasterDirectoryToSegmentDirectories(nil, agentRequest)
		Expect(err.Error()).To(ContainSubstring("Segment data directory /tmp/dataDir0 does not exist"))
	})

	It("errors when failing to backup the segment data directory", func() {
		utils.System.Rename = func(oldpath, newpath string) error {
			return errors.Errorf("failed to rename %s", oldpath)
		}

		agent := services.NewAgentServer(testExecutor, services.AgentConfig{})
		_, err := agent.CopyMasterDirectoryToSegmentDirectories(nil, agentRequest)
		Expect(err.Error()).To(ContainSubstring("Could not back up segment data directory"))
	})

	It("errors when failing to copy the master directory to segment directory", func() {
		testExecutor.LocalError = errors.New("failed to copy")

		agent := services.NewAgentServer(testExecutor, services.AgentConfig{})
		_, err := agent.CopyMasterDirectoryToSegmentDirectories(nil, agentRequest)
		Expect(err.Error()).To(ContainSubstring("Could not copy master data directory to segment data directory"))
	})

	It("errors when failing to copy the back up configuration files", func() {
		utils.System.Rename = func(oldpath, newpath string) error {
			if strings.Contains(oldpath, "postgresql.conf") {
				return errors.New("failed to rename")
			}
			return nil
		}

		agent := services.NewAgentServer(testExecutor, services.AgentConfig{})
		_, err := agent.CopyMasterDirectoryToSegmentDirectories(nil, agentRequest)
		Expect(err.Error()).To(ContainSubstring("Could not copy postgresql.conf from backup segment directory to segment data directory"))
	})

	It("errors when failing to delete master-specific files", func() {
		utils.System.RemoveAll = func(name string) error {
			return errors.New("failed to delete directory")
		}

		agent := services.NewAgentServer(testExecutor, services.AgentConfig{})
		_, err := agent.CopyMasterDirectoryToSegmentDirectories(nil, agentRequest)
		Expect(err.Error()).To(ContainSubstring("Could not remove gp_dbid from segment data directory"))
	})

	It("errors when failing to delete copy of master data directory", func() {
		utils.System.RemoveAll = func(name string) error {
			if name == "/tmp/masterDataDir" {
				return errors.New("failed to delete directory")
			}
			return nil
		}

		agent := services.NewAgentServer(testExecutor, services.AgentConfig{})
		_, err := agent.CopyMasterDirectoryToSegmentDirectories(nil, agentRequest)
		Expect(err.Error()).To(ContainSubstring("Could not delete copy of master data directory"))
	})

})
