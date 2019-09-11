package upgradestatus_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus/file"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

var _ = Describe("Upgradestatus/Seginstall", func() {
	var testLog *gbytes.Buffer

	BeforeEach(func() {
		// FIXME: redirect stdout/err to GingkoWriter instead of swallowing it
		_, _, testLog = testhelper.SetupTestLogger()
	})
	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
	})

	It("Reports PENDING if no directory exists", func() {
		stateChecker := upgradestatus.StateCheck{Path: "/fake/path", Step: idl.UpgradeSteps_UNKNOWN_STEP}
		upgradeStepStatus := stateChecker.GetStatus()
		Expect(upgradeStepStatus).To(Equal(idl.StepStatus_PENDING))
	})
	It("Reports RUNNING if statedir exists and contains inprogress file", func() {
		fakePath := "/fake/path"
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			if name == fakePath {
				return nil, nil
			}
			return nil, errors.New("unexpected Stat call")
		}
		utils.System.FilePathGlob = func(glob string) ([]string, error) {
			if glob == fakePath+"/*" {
				return []string{filepath.Join(fakePath, file.InProgress)}, nil
			}
			return nil, errors.New("didn't match expected glob pattern")
		}
		stateChecker := upgradestatus.StateCheck{Path: fakePath, Step: idl.UpgradeSteps_UNKNOWN_STEP}
		upgradeStepStatus := stateChecker.GetStatus()
		Expect(upgradeStepStatus).To(Equal(idl.StepStatus_RUNNING))
	})
	It("Reports FAILED if statedir exists and contains failed file", func() {
		fakePath := "/fake/path"
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			if name == fakePath {
				return nil, nil
			}
			return nil, errors.New("unexpected Stat call")
		}
		utils.System.FilePathGlob = func(glob string) ([]string, error) {
			if glob == fakePath+"/*" {
				return []string{filepath.Join(fakePath, file.Failed)}, nil
			}
			return nil, errors.New("didn't match expected glob pattern")
		}
		stateChecker := upgradestatus.StateCheck{Path: fakePath, Step: idl.UpgradeSteps_UNKNOWN_STEP}
		upgradeStepStatus := stateChecker.GetStatus()
		Expect(upgradeStepStatus).To(Equal(idl.StepStatus_FAILED))
	})

	It("logs an error if there is more than one file at the specified path", func() {
		overabundantDirectory := "/full/of/stuff"
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			if name == overabundantDirectory {
				return nil, nil
			}
			return nil, errors.New("unexpected Stat call")
		}

		utils.System.FilePathGlob = func(glob string) ([]string, error) {
			if glob == overabundantDirectory+"/*" {
				return []string{"first", "second"}, nil
			}
			return nil, errors.New("didn't match expected glob pattern")
		}
		stateChecker := upgradestatus.StateCheck{Path: overabundantDirectory, Step: idl.UpgradeSteps_UNKNOWN_STEP}
		upgradeStepStatus := stateChecker.GetStatus()

		// This is a little brittle, sorry...
		expectederr := fmt.Sprintf("%s has more than one file", overabundantDirectory)
		Expect(testLog).To(gbytes.Say(expectederr))

		// The installation should still be marked pending.
		Expect(upgradeStepStatus).To(Equal(idl.StepStatus_PENDING))
	})
})
