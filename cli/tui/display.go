package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/greenplum-db/gpupgrade/idl"
)

var UpgradeStepStatusDescription = map[idl.StepStatus]string{
	idl.StepStatus_UNKNOWN_STATUS: "UNKNOWN",
	idl.StepStatus_PENDING:        "PENDING",
	idl.StepStatus_RUNNING:        "IN_PROGRESS",
	idl.StepStatus_COMPLETE:       "COMPLETE",
	idl.StepStatus_FAILED:         "FAILED",
}

var UpgradeStepsDescription = map[idl.UpgradeSteps]string{
	idl.UpgradeSteps_UNKNOWN_STEP:           "unknown step value",
	idl.UpgradeSteps_CONFIG:                 "retrieving configs",
	idl.UpgradeSteps_START_AGENTS:           "starting agents",
	idl.UpgradeSteps_INIT_CLUSTER:           "stp value unimplemented",
	idl.UpgradeSteps_CONVERT_MASTER:         "stp value unimplemented",
	idl.UpgradeSteps_SHUTDOWN_CLUSTERS:      "stp value unimplemented",
	idl.UpgradeSteps_COPY_MASTER:            "stp value unimplemented",
	idl.UpgradeSteps_CONVERT_PRIMARIES:      "stp value unimplemented",
	idl.UpgradeSteps_VALIDATE_START_CLUSTER: "stp value unimplemented",
	idl.UpgradeSteps_RECONFIGURE_PORTS:      "stp value unimplemented",
}

var displayed = make(map[idl.UpgradeSteps]bool)

// OutputStatus prints out all COMPLETED||FAILED steps and the current pending one
func OutputStatus(status *idl.UpgradeStepStatus) string {
	var s strings.Builder

	completeOrFailed := status.GetStatus() == idl.StepStatus_COMPLETE ||
		status.GetStatus() == idl.StepStatus_FAILED
	haveNotDisplayedFinalStepState := !displayed[status.GetStep()]

	if completeOrFailed && haveNotDisplayedFinalStepState {
		fmt.Fprintf(&s, "%s\n",
			getDisplayLine(status.GetStep(), status.GetStatus()))
		displayed[status.GetStep()] = true
	} else if !completeOrFailed {
		fmt.Fprintf(&s, "%s\r", getDisplayLine(status.GetStep(), status.GetStatus()))
	}

	return s.String()
}

// GetDisplayLine returns a string that justifies the step and its status
// stepString...(pad to max stepString)(blanks)[status](trailing blanks)
func getDisplayLine(step idl.UpgradeSteps, status idl.StepStatus) string {

	stepString := getStepString(step)
	statusString := getStepStatusString(status)

	numBrackets := 2
	numDots := 3
	maxLeftEndBackoff := getMaxStepStringLength()
	maxRightEndBackoff := getMaxStepStatusStringLength()

	stepStringPad := maxLeftEndBackoff - len(stepString)
	numBlanks := 80 - numBrackets - numDots - maxLeftEndBackoff - maxRightEndBackoff
	numTrailingBlanks := maxRightEndBackoff - len(statusString)
	if numBlanks < 0 {
		stepStringPad = 0
		numBlanks = 0
	}

	stepStringSpaces := strings.Repeat(" ", stepStringPad)
	numBlanksSpaces := strings.Repeat(" ", numBlanks)
	trailingSpaces := strings.Repeat(" ", numTrailingBlanks)

	var s strings.Builder
	fmt.Fprintf(&s, "%s...%s%s[%s]%s", stepString, stepStringSpaces, numBlanksSpaces, statusString, trailingSpaces)

	return s.String()

}

func getMaxStepStringLength() int {
	maxLen := float64(len(getStepString(idl.UpgradeSteps(math.MaxInt32))))
	for step, _ := range idl.UpgradeSteps_name {
		maxLen = math.Max(
			float64(maxLen),
			float64(len(getStepString(idl.UpgradeSteps(step)))),
		)
	}

	return int(maxLen)
}

func getMaxStepStatusStringLength() int {
	maxLen := float64(len(getStepStatusString(idl.StepStatus(math.MaxInt32))))
	for step, _ := range idl.StepStatus_name {
		maxLen = math.Max(
			float64(maxLen),
			float64(len(getStepStatusString(idl.StepStatus(step)))),
		)
	}

	return int(maxLen)
}

func getStepString(step idl.UpgradeSteps) string {
	stepDescription := UpgradeStepsDescription[step]
	if stepDescription != "" {
		return stepDescription
	}
	return "invalid step value"
}

func getStepStatusString(status idl.StepStatus) string {
	statusDescription := UpgradeStepStatusDescription[status]
	if statusDescription != "" {
		return statusDescription
	}
	return "INVALID"
}
