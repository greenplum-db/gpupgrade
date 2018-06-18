package services

import (
	"path/filepath"
	"strings"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/net/context"
)

// Step represents a single step during upgrade status (e.g. primary conversion,
// check config, etc.).
type Step struct {
	// Name is a human-readable description of the Step, usually following the
	// invocation on the command line. It must additionally be a valid directory
	// name for all supported filesystems.
	Name string

	// StepCode is the gRPC code corresponding to this particular Step.
	StepCode pb.UpgradeSteps

	// getStatus is the private implementation of the Status method.
	getStatus func(s Step, h *Hub) *pb.UpgradeStepStatus
}

// Status retrieves the UpgradeStepStatus (failed, completed, etc.) for this
// Step on a given Hub.
func (s Step) Status(h *Hub) *pb.UpgradeStepStatus {
	return s.getStatus(s, h)
}

// StatePath returns the directory where the state for this Step is kept (if
// applicable; not all Steps use a state directory in their implementation). It
// is currently computed using the Hub's state directory and the Step name.
func (s Step) StatePath(h *Hub) string {
	path := filepath.Join(h.conf.StateDir, s.Name)
	gplog.Debug("looking for %s state at %s", s.Name, path)
	return path
}

/*
 * getStatus() Implementations
 */

// stateCheckStatus uses a NewStateCheck object to retrieve status; it's the
// most general getStatus() implementation.
func stateCheckStatus(s Step, h *Hub) *pb.UpgradeStepStatus {
	state := upgradestatus.NewStateCheck(s.StatePath(h), s.StepCode)
	return state.GetStatus()
}

// initStatus gets its status by checking for the existence of a new cluster
// config.
func initStatus(s Step, h *Hub) *pb.UpgradeStepStatus {
	status := h.GetPrepareNewClusterConfigStatus()
	return status
}

// shutdownStatus checks whether all clusters have been stopped.
func shutdownStatus(s Step, h *Hub) *pb.UpgradeStepStatus {
	state := upgradestatus.NewShutDownClusters(s.StatePath(h), h.commandExecer)
	return state.GetStatus()
}

// pgUpgradeStatus checks the pg_upgrade progress files for its status.
func pgUpgradeStatus(s Step, h *Hub) *pb.UpgradeStepStatus {
	status := &pb.UpgradeStepStatus{
		Step: s.StepCode,
	}
	// We don't need to check the pg_upgrade status if there's no configuration yet
	checkConfigStep := Step{upgradestatus.CONFIG, pb.UpgradeSteps_CHECK_CONFIG, stateCheckStatus}
	if checkConfigStep.getStatus(checkConfigStep, h).Status != pb.StepStatus_COMPLETE {
		status.Status = pb.StepStatus_PENDING
		return status
	}
	state := upgradestatus.NewPGUpgradeStatusChecker(s.StatePath(h), h.clusterPair.OldCluster.GetDirForContent(-1), h.commandExecer)
	return state.GetStatus()
}

// conversionStatus queries all segments for their upgrade status and
// consolidates them into a single pass/fail.
func conversionStatus(s Step, h *Hub) *pb.UpgradeStepStatus {
	status := &pb.UpgradeStepStatus{
		Step: s.StepCode,
	}
	// We can't check the status of agent processes if the agents haven't been started yet
	startAgentsStep := Step{upgradestatus.START_AGENTS, pb.UpgradeSteps_PREPARE_START_AGENTS, stateCheckStatus}
	if startAgentsStep.getStatus(startAgentsStep, h).Status != pb.StepStatus_COMPLETE {
		status.Status = pb.StepStatus_PENDING
		return status
	}
	// We can't determine the actual status if there's an error, so we log it and return PENDING
	conversionStatus, err := h.StatusConversion(nil, &pb.StatusConversionRequest{})
	if err != nil {
		gplog.Error("Could not get primary conversion status: %s", err)
		status.Status = pb.StepStatus_PENDING
		return status
	}
	statuses := strings.Join(conversionStatus.GetConversionStatuses(), "\n")
	if strings.Contains(statuses, "FAILED") {
		status.Status = pb.StepStatus_FAILED
	} else if strings.Contains(statuses, "RUNNING") {
		status.Status = pb.StepStatus_RUNNING
	} else if strings.Contains(statuses, "COMPLETE") {
		status.Status = pb.StepStatus_COMPLETE
	} else {
		status.Status = pb.StepStatus_PENDING
	}
	return status
}

func (h *Hub) StatusUpgrade(ctx context.Context, in *pb.StatusUpgradeRequest) (*pb.StatusUpgradeReply, error) {
	gplog.Info("starting StatusUpgrade")

	steps := [...]Step{
		{"check-config", pb.UpgradeSteps_CHECK_CONFIG, stateCheckStatus},
		{"seginstall", pb.UpgradeSteps_SEGINSTALL, stateCheckStatus},
		{"init-cluster", pb.UpgradeSteps_PREPARE_INIT_CLUSTER, initStatus},
		{"gpstop", pb.UpgradeSteps_STOPPED_CLUSTER, shutdownStatus},
		{"pg_upgrade", pb.UpgradeSteps_MASTERUPGRADE, pgUpgradeStatus},
		{"start-agents", pb.UpgradeSteps_PREPARE_START_AGENTS, stateCheckStatus},
		{"share-oids", pb.UpgradeSteps_SHARE_OIDS, stateCheckStatus},
		{"validate-start-cluster", pb.UpgradeSteps_VALIDATE_START_CLUSTER, stateCheckStatus},
		{"convert-primaries", pb.UpgradeSteps_CONVERT_PRIMARIES, conversionStatus},
		{"reconfigure-ports", pb.UpgradeSteps_RECONFIGURE_PORTS, stateCheckStatus},
	}

	statuses := make([]*pb.UpgradeStepStatus, len(steps))

	for i, desc := range steps {
		gplog.Info("Checking %s...", desc.Name)
		statuses[i] = desc.Status(h)
	}

	return &pb.StatusUpgradeReply{
		ListOfUpgradeStepStatuses: statuses,
	}, nil
}

func (h *Hub) GetPrepareNewClusterConfigStatus() *pb.UpgradeStepStatus {
	/* Treat all stat failures as cannot find file. Conceal worse failures atm.*/
	_, err := utils.System.Stat(GetNewConfigFilePath(h.conf.StateDir))

	if err != nil {
		gplog.Debug("%v", err)
		return &pb.UpgradeStepStatus{
			Step:   pb.UpgradeSteps_PREPARE_INIT_CLUSTER,
			Status: pb.StepStatus_PENDING,
		}
	}

	return &pb.UpgradeStepStatus{
		Step:   pb.UpgradeSteps_PREPARE_INIT_CLUSTER,
		Status: pb.StepStatus_COMPLETE,
	}
}
