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

func TestOutputStatus(t *testing.T) {
	var includedValid1 = map[idl.UpgradeSteps]bool{}
	includedValid1[idl.UpgradeSteps_START_AGENTS] = true

	var includedValid2 = map[idl.UpgradeSteps]bool{}
	includedValid2[idl.UpgradeSteps_CONFIG] = true

	type args struct {
		status *idl.UpgradeStepStatus
	}
	validStatus1 := &idl.UpgradeStepStatus{
		Step:   idl.UpgradeSteps_START_AGENTS,
		Status: idl.StepStatus_RUNNING,
	}

	validStatus2 := &idl.UpgradeStepStatus{
		Step:   idl.UpgradeSteps_CONFIG,
		Status: idl.StepStatus_COMPLETE,
	}

	tests := []struct {
		name string
		args args
		want string
	}{
		{
			"OutputStatus when using a valid input that is in progress",
			args{validStatus1},
			`^starting agents[.]{3}\s*\[IN_PROGRESS\]\s*\r$`,
		},
		{
			"OutputStatus when using a valid input that is completed",
			args{validStatus2},
			`^retrieving configs[.]{3}\s*\[COMPLETE\]\s*\n$`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := setupTest(t)

			got := OutputStatus(tt.args.status)
			g.Expect(got).To(MatchRegexp(tt.want))
		})
	}
}

func Test_getDisplayLine(t *testing.T) {
	type args struct {
		step   idl.UpgradeSteps
		status idl.StepStatus
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"getDisplayLine when using a supported step and status",
			args{step: idl.UpgradeSteps_START_AGENTS, status: idl.StepStatus_COMPLETE},
			`^starting agents[.]{3}\s*\[COMPLETE\]\s*$`,
		},
		{"getDisplayLine when unknown step and status",
			args{step: idl.UpgradeSteps_UNKNOWN_STEP, status: idl.StepStatus_UNKNOWN_STATUS},
			`^unknown step value[.]{3}\s*\[UNKNOWN\]\s*$`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := setupTest(t)

			got := getDisplayLine(tt.args.step, tt.args.status)
			g.Expect(got).To(MatchRegexp(tt.want))
			g.Expect(len(got)).To(Equal(80))
		})
	}
}
