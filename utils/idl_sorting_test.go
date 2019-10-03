package utils

import (
	"math"
	"testing"

	"github.com/greenplum-db/gpupgrade/idl"
)

var (
	len0StepStatus     = []*idl.UpgradeStepStatus{}
	len1StepStatus     = []*idl.UpgradeStepStatus{{Step: idl.UpgradeSteps_UNKNOWN_STEP}}
	len2StepStatus1lt2 = []*idl.UpgradeStepStatus{
		{Step: idl.UpgradeSteps_UNKNOWN_STEP},
		{Step: idl.UpgradeSteps_CONFIG},
	}
	len2StepStatus2lt1 = []*idl.UpgradeStepStatus{
		{Step: idl.UpgradeSteps_CONFIG},
		{Step: idl.UpgradeSteps_UNKNOWN_STEP},
	}
	len2StepStatusOob = []*idl.UpgradeStepStatus{
		{Step: idl.UpgradeSteps_CONFIG},
		{Step: math.MaxInt32}, //large, out-of-band value
	}
)

func TestStepStatuses_Len(t *testing.T) {
	tests := []struct {
		name string
		s    StepStatuses
		want int
	}{
		{
			"length_0",
			len0StepStatus,
			0,
		},
		{
			"length_1",
			len1StepStatus,
			1,
		},
		{
			"length_2",
			len2StepStatus1lt2,
			2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Len(); got != tt.want {
				t.Errorf("StepStatuses.Len() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStepStatuses_Less(t *testing.T) {
	type args struct {
		i int
		j int
	}
	tests := []struct {
		name string
		s    StepStatuses
		args args
		want bool
	}{
		{
			"basic",
			len2StepStatus1lt2,
			args{0, 1},
			true,
		},
		{
			"reverse",
			len2StepStatus2lt1,
			args{0, 1},
			false,
		},
		{
			"out-of-band",
			len2StepStatusOob,
			args{1, 0},
			true,
		},
		{
			"out-of-band-reverse",
			len2StepStatusOob,
			args{0, 1},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Less(tt.args.i, tt.args.j); got != tt.want {
				t.Errorf("StepStatuses.Less() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStepStatuses_Swap(t *testing.T) {
	stepStatuses := StepStatuses([]*idl.UpgradeStepStatus{
		{Step: idl.UpgradeSteps_UNKNOWN_STEP},
		{Step: idl.UpgradeSteps_CONFIG},
	})
	expected := StepStatuses([]*idl.UpgradeStepStatus{
		{Step: idl.UpgradeSteps_CONFIG},
		{Step: idl.UpgradeSteps_UNKNOWN_STEP},
	})

	stepStatuses.Swap(0, 1)
	for i := range stepStatuses {
		if stepStatuses[i].GetStep() != expected[i].GetStep() {
			t.Errorf("StepStatuses.Swap() for %v got %v, wanted %v", i, stepStatuses[i], expected[i])
		}
	}
}

func TestPrimaryStatuses_Len(t *testing.T) {
	tests := []struct {
		name string
		s    PrimaryStatuses
		want int
	}{
		{
			"empty",
			[]*idl.PrimaryStatus{},
			0,
		},
		{
			"len1",
			[]*idl.PrimaryStatus{{Dbid: 1}},
			1,
		},
		{
			"len2",
			[]*idl.PrimaryStatus{{Dbid: 1}, {Dbid: 2}},
			2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Len(); got != tt.want {
				t.Errorf("PrimaryStatuses.Len() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrimaryStatuses_Less(t *testing.T) {
	lessThan := PrimaryStatuses([]*idl.PrimaryStatus{{Dbid: 1}, {Dbid: 2}})
	if got := lessThan.Less(0, 1); got != true {
		t.Errorf("PrimaryStatuses.Less() = %v, want %v", got, true)
	}

	greaterThan := PrimaryStatuses([]*idl.PrimaryStatus{{Dbid: 2}, {Dbid: 1}})
	if got := greaterThan.Less(0, 1); got != false {
		t.Errorf("PrimaryStatuses.Less() = %v, want %v", got, false)
	}
}

func TestPrimaryStatuses_Swap(t *testing.T) {
	primaryStatuses := PrimaryStatuses([]*idl.PrimaryStatus{{Dbid: 1}, {Dbid: 2}})
	expected := []*idl.PrimaryStatus{{Dbid: 2}, {Dbid: 1}}
	primaryStatuses.Swap(0, 1)
	for i := range primaryStatuses {
		if primaryStatuses[i].GetDbid() != expected[i].GetDbid() {
			t.Errorf("PrimaryStatuses.Swap() index %v got %v expected %v", i, primaryStatuses[i], expected[i])
		}
	}
}
