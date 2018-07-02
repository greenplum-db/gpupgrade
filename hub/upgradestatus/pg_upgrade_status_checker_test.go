package upgradestatus_test

import (
	"io/ioutil"
	"os"
	"strings"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/pkg/errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("pg_upgrade status checker", func() {
	var (
		testExecutor *testhelper.TestExecutor
	)

	BeforeEach(func() {
		testhelper.SetupTestLogger() // extend to capture the values in a var if future tests need it

		testExecutor = &testhelper.TestExecutor{}
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
	})

	It("If pg_upgrade dir does not exist, return status of PENDING", func() {
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}
		utils.System.IsNotExist = func(error) bool {
			return true
		}
		subject := upgradestatus.NewPGUpgradeStatusChecker(upgradestatus.MASTER, "/tmp", "", testExecutor)
		status := subject.GetStatus()
		Expect(status.Status).To(Equal(pb.StepStatus_PENDING))

	})

	It("If pg_upgrade is running, return status of RUNNING", func() {
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}
		utils.System.IsNotExist = func(error) bool {
			return false
		}

		testExecutor.LocalOutput = "I'm running"

		subject := upgradestatus.NewPGUpgradeStatusChecker(upgradestatus.MASTER, "/tmp", "", testExecutor)
		status := subject.GetStatus()
		Expect(status.Status).To(Equal(pb.StepStatus_RUNNING))
	})

	It("If pg_upgrade is not running and .done files exist and contain the string "+
		"'Upgrade completed',return status of COMPLETED", func() {
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}
		utils.System.IsNotExist = func(error) bool {
			return false
		}

		testExecutor.LocalError = errors.New("exit status 1")

		utils.System.FilePathGlob = func(glob string) ([]string, error) {
			if strings.Contains(glob, "inprogress") {
				return nil, errors.New("fake error")
			} else if strings.Contains(glob, "done") {
				return []string{"found something"}, nil
			}

			return nil, errors.New("Test not configured for this glob.")
		}
		utils.System.Stat = func(filename string) (os.FileInfo, error) {
			if strings.Contains(filename, "found something") {
				return &testutils.FakeFileInfo{}, nil
			}
			return nil, nil
		}
		utils.System.Open = func(name string) (*os.File, error) {
			// Temporarily create a file that we can read as a real file descriptor
			fd, err := ioutil.TempFile("/tmp", "hub_status_upgrade_test")
			Expect(err).To(BeNil())

			filename := fd.Name()
			fd.WriteString("12312312;Upgrade complete;\n")
			fd.Close()
			return os.Open(filename)
		}

		subject := upgradestatus.NewPGUpgradeStatusChecker(upgradestatus.MASTER, "/tmp", "/data/dir", testExecutor)
		status := subject.GetStatus()
		Expect(status.Status).To(Equal(pb.StepStatus_COMPLETE))

		Expect(testExecutor.LocalCommands).To(Equal([]string{"pgrep pg_upgrade | grep --old-datadir=/data/dir"}))
	})

	// We are assuming that no inprogress actually exists in the path we're using,
	// so we don't need to mock the checks out.
	It("If pg_upgrade not running and no .inprogress or .done files exists, "+
		"return status of FAILED", func() {
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}
		utils.System.IsNotExist = func(error) bool {
			return false
		}

		testExecutor.LocalError = errors.New("pg_upgrade failed")

		subject := upgradestatus.NewPGUpgradeStatusChecker(upgradestatus.MASTER, "/tmp", "", testExecutor)
		status := subject.GetStatus()
		Expect(status.Status).To(Equal(pb.StepStatus_FAILED))
	})
})
