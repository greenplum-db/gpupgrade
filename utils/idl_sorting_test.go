package utils

import (
	"math"
	"testing"

	"github.com/greenplum-db/gpupgrade/idl"
)

var (
	len0StepStatus = []*idl.UpgradeStepStatus{}
	len1StepStatus = []*idl.UpgradeStepStatus{
		{
			Step: idl.UpgradeSteps_UNKNOWN_STEP,
		},
	}
	len2StepStatus_1lt2 = []*idl.UpgradeStepStatus{
		{
			Step: idl.UpgradeSteps_UNKNOWN_STEP,
		},
		{
			Step: idl.UpgradeSteps_CONFIG,
		},
	}
	len2StepStatus_2lt1 = []*idl.UpgradeStepStatus{
		{
			Step: idl.UpgradeSteps_CONFIG,
		},
		{
			Step: idl.UpgradeSteps_UNKNOWN_STEP,
		},
	}
	len2StepStatus_oob = []*idl.UpgradeStepStatus{
		{
			Step: idl.UpgradeSteps_CONFIG,
		},
		{
			Step: math.MaxInt32, //large, out-of-band value
		},
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
			len2StepStatus_1lt2,
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
			len2StepStatus_1lt2,
			args{0, 1},
			true,
		},
		{
			"reverse",
			len2StepStatus_2lt1,
			args{0, 1},
			false,
		},
		{
			"out-of-band",
			len2StepStatus_oob,
			args{1, 0},
			true,
		},
		{
			"out-of-band-reverse",
			len2StepStatus_oob,
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
	type args struct {
		i int
		j int
	}
	tests := []struct {
		name string
		sIn  StepStatuses
		sOut StepStatuses
		args args
	}{
		{
			"basic",
			[]*idl.UpgradeStepStatus{
				{
					Step: idl.UpgradeSteps_UNKNOWN_STEP,
				},
				{
					Step: idl.UpgradeSteps_CONFIG,
				},
			},
			[]*idl.UpgradeStepStatus{
				{
					Step: idl.UpgradeSteps_CONFIG,
				},
				{
					Step: idl.UpgradeSteps_UNKNOWN_STEP,
				},
			},
			args{0, 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.sIn.Swap(tt.args.i, tt.args.j)
			for i := range tt.sIn {
				if tt.sIn[i].GetStep() != tt.sOut[i].GetStep() {
					t.Errorf("StepStatuses.Swap() for %v got %v, wanted %v", i, tt.sIn[i], tt.sOut[i])
				}
			}
		})
	}
}

// XXX: do we really just want to be using well-formed data from gp_segment_configuration here?
var (
	len0PrimaryStatus = []*idl.PrimaryStatus{}
	len1PrimaryStatus = []*idl.PrimaryStatus{
		{
			Dbid: 1,
		},
	}
	len2PrimaryStatus_1lt2 = []*idl.PrimaryStatus{
		{
			Dbid: 1,
		},
		{
			Dbid: 2,
		},
	}
	len2PrimaryStatus_2lt1 = []*idl.PrimaryStatus{
		{
			Dbid: 2,
		},
		{
			Dbid: 1,
		},
	}
)

func TestPrimaryStatuses_Len(t *testing.T) {
	tests := []struct {
		name string
		s    PrimaryStatuses
		want int
	}{
		{
			"empty",
			len0PrimaryStatus,
			0,
		},
		{
			"len1",
			len1PrimaryStatus,
			1,
		},
		{
			"len2",
			len2PrimaryStatus_1lt2,
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
	type args struct {
		i int
		j int
	}
	tests := []struct {
		name string
		s    PrimaryStatuses
		args args
		want bool
	}{
		{
			"basic",
			len2PrimaryStatus_1lt2,
			args{0, 1},
			true,
		},
		{
			"reverse",
			len2PrimaryStatus_2lt1,
			args{0, 1},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Less(tt.args.i, tt.args.j); got != tt.want {
				t.Errorf("PrimaryStatuses.Less() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrimaryStatuses_Swap(t *testing.T) {
	type args struct {
		i int
		j int
	}
	tests := []struct {
		name string
		sIn  PrimaryStatuses
		sOut PrimaryStatuses
		args args
	}{
		{"basic",
			[]*idl.PrimaryStatus{
				{
					Status:   idl.StepStatus_FAILED,
					Dbid:     1,
					Content:  3,
					Hostname: "localhost",
				},
				{
					Status:   idl.StepStatus_RUNNING,
					Dbid:     2,
					Content:  4,
					Hostname: "localhost",
				},
			},
			[]*idl.PrimaryStatus{
				{
					Status:   idl.StepStatus_RUNNING,
					Dbid:     2,
					Content:  4,
					Hostname: "localhost",
				},
				{
					Status:   idl.StepStatus_FAILED,
					Dbid:     1,
					Content:  3,
					Hostname: "localhost",
				},
			},
			args{0, 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.sIn.Swap(tt.args.i, tt.args.j)
			for i := range tt.sIn {
				if tt.sIn[i].GetDbid() != tt.sOut[i].GetDbid() {
					t.Errorf("PrimaryStatuses.Swap() index %v got %v expected %v", i, tt.sIn[i], tt.sOut[i])

				}
			}

		})
	}
}
