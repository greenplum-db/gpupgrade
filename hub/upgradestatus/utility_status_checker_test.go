package upgradestatus_test

import (
	"os"
	"strings"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus/file"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/operating"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/pkg/errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("utility status checker", func() {
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
		status := upgradestatus.SegmentConversionStatus("/tmp", "", testExecutor)
		Expect(status).To(Equal(pb.StepStatus_PENDING))

	})

	It("If pg_upgrade is running, return status of RUNNING", func() {
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}
		utils.System.IsNotExist = func(error) bool {
			return false
		}

		testExecutor.LocalOutput = "I'm running"

		utils.System.FilePathGlob = func(glob string) ([]string, error) {
			if strings.Contains(glob, ".inprogress") {
				return []string{"found something"}, nil
			}
			return nil, errors.New("Test not configured for this glob.")
		}

		status := upgradestatus.SegmentConversionStatus("/tmp", "", testExecutor)
		Expect(status).To(Equal(pb.StepStatus_RUNNING))
	})

	It("If pg_upgrade is not running and .done files exist and contain the string "+
		"'Upgrade complete',return status of COMPLETED", func() {
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
		testhelper.MockFileContents("Upgrade complete")
		defer operating.InitializeSystemFunctions()
		status := upgradestatus.SegmentConversionStatus("/tmp", "/data/dir", testExecutor)
		Expect(testExecutor.LocalCommands).To(Equal([]string{"pgrep -f pg_upgrade | grep /data/dir"}))
		Expect(status).To(Equal(pb.StepStatus_COMPLETE))
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

		status := upgradestatus.SegmentConversionStatus("/tmp", "", testExecutor)
		Expect(status).To(Equal(pb.StepStatus_FAILED))
	})

	It("If gpstop dir does not exist, return status of PENDING", func() {
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}
		utils.System.IsNotExist = func(error) bool {
			return true
		}
		status := upgradestatus.ClusterShutdownStatus("/tmp", testExecutor)
		Expect(status).To(Equal(pb.StepStatus_PENDING))

	})
	It("If gpstop is running, return status of RUNNING", func() {
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}
		utils.System.IsNotExist = func(error) bool {
			return false
		}

		testExecutor.LocalOutput = "I'm running"

		utils.System.FilePathGlob = func(glob string) ([]string, error) {
			if strings.Contains(glob, file.InProgress) {
				return []string{"found something"}, nil
			}
			return nil, errors.New("Test not configured for this glob.")
		}
		status := upgradestatus.ClusterShutdownStatus("/tmp", testExecutor)
		Expect(status).To(Equal(pb.StepStatus_RUNNING))
	})
	It("If gpstop is not running and .complete files exist and contain the string "+
		"'Upgrade complete',return status of COMPLETED", func() {
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}
		utils.System.IsNotExist = func(error) bool {
			return false
		}

		testExecutor.LocalError = errors.New("exit status 1")

		utils.System.FilePathGlob = func(glob string) ([]string, error) {
			if strings.Contains(glob, file.InProgress) {
				return nil, errors.New("fake error")
			} else if strings.Contains(glob, file.Complete) {
				return []string{"old stop complete", "new stop complete"}, nil
			}

			return nil, errors.New("Test not configured for this glob.")
		}
		utils.System.Stat = func(filename string) (os.FileInfo, error) {
			if strings.Contains(filename, "found something") {
				return &testutils.FakeFileInfo{}, nil
			}
			return nil, nil
		}
		testhelper.MockFileContents("Upgrade complete")
		defer operating.InitializeSystemFunctions()
		status := upgradestatus.ClusterShutdownStatus("/tmp", testExecutor)
		Expect(status).To(Equal(pb.StepStatus_COMPLETE))
	})
	// We are assuming that no inprogress actually exists in the path we're using,
	// so we don't need to mock the checks out.
	It("If gpstop not running and no .inprogress or .complete files exists, "+
		"return status of FAILED", func() {
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}
		utils.System.IsNotExist = func(error) bool {
			return false
		}

		testExecutor.LocalError = errors.New("gpstop failed")

		status := upgradestatus.ClusterShutdownStatus("/tmp", testExecutor)
		Expect(status).To(Equal(pb.StepStatus_FAILED))
	})
})
