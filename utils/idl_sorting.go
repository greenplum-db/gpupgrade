package utils

import "github.com/greenplum-db/gpupgrade/idl"

/*
 * This map, and the associated UpgradeStepStatus sorting functions below,
 * enable sorting gpupgrade status.
 *
 * See https://developers.google.com/protocol-buffers/docs/reference/go-generated#enum
 *    From this, we learn that a .proto Enum will be an int32, and will generate
 *    certain golang symbols(like enumName_name), which we use below.  In addition,
 *    we learn that out-of-band enum values can, in fact, be returned if the sender
 *    so chooses.
 *
 */

type StepStatuses []*idl.UpgradeStepStatus

func (s StepStatuses) Len() int {
	return len(s)
}

func (s StepStatuses) Less(i, j int) bool {
	// see above for why this is automagically kept in touch with the .proto file
	iStep := s[i].GetStep()
	jStep := s[j].GetStep()
	if idl.UpgradeSteps_name[int32(iStep)] == "" {
		iStep = idl.UpgradeSteps_UNKNOWN_STEP
	}
	if idl.UpgradeSteps_name[int32(jStep)] == "" {
		jStep = idl.UpgradeSteps_UNKNOWN_STEP
	}
	return int32(iStep) < int32(jStep)
}

func (s StepStatuses) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

type PrimaryStatuses []*idl.PrimaryStatus

func (s PrimaryStatuses) Len() int {
	return len(s)
}

func (s PrimaryStatuses) Less(i, j int) bool {
	return s[i].Dbid < s[j].Dbid
}

func (s PrimaryStatuses) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
