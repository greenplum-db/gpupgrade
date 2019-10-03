package tui

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gpupgrade/idl"
	. "github.com/onsi/gomega"
)

func setupTest(t *testing.T) *GomegaWithT {
	g := NewGomegaWithT(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	return g
}

func Test_OutputStatus_WithRunningInput(t *testing.T) {
	g := setupTest(t)
	upgradeStepStatus := &idl.UpgradeStepStatus{
		Step:   idl.UpgradeSteps_START_AGENTS,
		Status: idl.StepStatus_RUNNING,
	}

	got := OutputStatus(upgradeStepStatus)
	g.Expect(got).To(MatchRegexp(`^starting agents[.]{3}\s*\[IN_PROGRESS\]\s*\r$`))
}

func Test_OutputStatus_WithCompleteStatus(t *testing.T) {
	g := setupTest(t)
	unknownUpgradeStepStatus := &idl.UpgradeStepStatus{
		Step:   idl.UpgradeSteps_CONFIG,
		Status: idl.StepStatus_COMPLETE,
	}

	got := OutputStatus(unknownUpgradeStepStatus)
	g.Expect(got).To(MatchRegexp(`^retrieving configs[.]{3}\s*\[COMPLETE\]\s*\n$`))
}

func Test_getDisplayLine_UsingKnownStepStatus(t *testing.T) {
	g := setupTest(t)

	got := getDisplayLine(idl.UpgradeSteps_START_AGENTS, idl.StepStatus_COMPLETE)
	g.Expect(got).To(MatchRegexp(`^starting agents[.]{3}\s*\[COMPLETE\]\s*$`))
	g.Expect(len(got)).To(Equal(80))
}

func Test_getDisplayLine_UsingUnknownStepStatus(t *testing.T) {
	g := setupTest(t)

	got := getDisplayLine(idl.UpgradeSteps_UNKNOWN_STEP, idl.StepStatus_UNKNOWN_STATUS)
	g.Expect(got).To(MatchRegexp(`^unknown step value[.]{3}\s*\[UNKNOWN\]\s*$`))
	g.Expect(len(got)).To(Equal(80))
}
