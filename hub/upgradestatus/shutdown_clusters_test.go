package upgradestatus_test

import (
	"os"
	"strings"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/pkg/errors"
)

var _ = Describe("hub", func() {
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

	Describe("ShutDownClusters", func() {
		It("If gpstop dir does not exist, return status of PENDING", func() {
			utils.System.Stat = func(name string) (os.FileInfo, error) {
				return nil, nil
			}
			utils.System.IsNotExist = func(error) bool {
				return true
			}
			subject := upgradestatus.NewShutDownClusters("/tmp", testExecutor)
			status := subject.GetStatus()
			Expect(status.Status).To(Equal(pb.StepStatus_PENDING))

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
				if strings.Contains(glob, "in.progress") {
					return []string{"found something"}, nil
				}
				return nil, errors.New("Test not configured for this glob.")
			}
			subject := upgradestatus.NewShutDownClusters("/tmp", testExecutor)
			status := subject.GetStatus()
			Expect(status.Status).To(Equal(pb.StepStatus_RUNNING))
		})
		It("If gpstop is not running and .complete files exist and contain the string "+
			"'Upgrade completed',return status of COMPLETED", func() {
			utils.System.Stat = func(name string) (os.FileInfo, error) {
				return nil, nil
			}
			utils.System.IsNotExist = func(error) bool {
				return false
			}

			testExecutor.LocalError = errors.New("exit status 1")

			utils.System.FilePathGlob = func(glob string) ([]string, error) {
				if strings.Contains(glob, "in.progress") {
					return nil, errors.New("fake error")
				} else if strings.Contains(glob, "complete") {
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
			subject := upgradestatus.NewShutDownClusters("/tmp", testExecutor)
			status := subject.GetStatus()
			Expect(status.Status).To(Equal(pb.StepStatus_COMPLETE))
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

			subject := upgradestatus.NewShutDownClusters("/tmp", testExecutor)
			status := subject.GetStatus()
			Expect(status.Status).To(Equal(pb.StepStatus_FAILED))
		})
	})
})
