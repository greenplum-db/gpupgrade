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

func (h *Hub) StatusUpgrade(ctx context.Context, in *pb.StatusUpgradeRequest) (*pb.StatusUpgradeReply, error) {
	gplog.Info("starting StatusUpgrade")

	checkconfigStatePath := filepath.Join(h.conf.StateDir, "check-config")
	checkconfigState := upgradestatus.NewStateCheck(checkconfigStatePath, pb.UpgradeSteps_CHECK_CONFIG)
	// XXX why do we ignore the error?
	checkconfigStatus, _ := checkconfigState.GetStatus()

	prepareInitStatus, _ := h.GetPrepareNewClusterConfigStatus()

	seginstallStatePath := filepath.Join(h.conf.StateDir, "seginstall")
	gplog.Debug("looking for seginstall state at %s", seginstallStatePath)
	seginstallState := upgradestatus.NewStateCheck(seginstallStatePath, pb.UpgradeSteps_SEGINSTALL)
	seginstallStatus, _ := seginstallState.GetStatus()

	gpstopStatePath := filepath.Join(h.conf.StateDir, "gpstop")
	gplog.Debug("looking for gpstop state at %s", gpstopStatePath)
	clusterPair := upgradestatus.NewShutDownClusters(gpstopStatePath, h.commandExecer)
	shutdownClustersStatus, _ := clusterPair.GetStatus()

	pgUpgradePath := filepath.Join(h.conf.StateDir, "pg_upgrade")
	gplog.Debug("looking for pg_upgrade state at %s", pgUpgradePath)
	convertMaster := upgradestatus.NewPGUpgradeStatusChecker(pgUpgradePath, h.clusterPair.OldCluster.GetDirForContent(-1), h.commandExecer)
	masterUpgradeStatus, _ := convertMaster.GetStatus()

	startAgentsStatePath := filepath.Join(h.conf.StateDir, "start-agents")
	gplog.Debug("looking for start-agents state at %s", startAgentsStatePath)
	prepareStartAgentsState := upgradestatus.NewStateCheck(startAgentsStatePath, pb.UpgradeSteps_PREPARE_START_AGENTS)
	startAgentsStatus, _ := prepareStartAgentsState.GetStatus()

	shareOidsPath := filepath.Join(h.conf.StateDir, "share-oids")
	shareOidsState := upgradestatus.NewStateCheck(shareOidsPath, pb.UpgradeSteps_SHARE_OIDS)
	shareOidsStatus, _ := shareOidsState.GetStatus()

	validateStartClusterPath := filepath.Join(h.conf.StateDir, "validate-start-cluster")
	validateStartClusterState := upgradestatus.NewStateCheck(validateStartClusterPath, pb.UpgradeSteps_VALIDATE_START_CLUSTER)
	validateStartClusterStatus, _ := validateStartClusterState.GetStatus()

	conversionStatus, _ := h.StatusConversion(nil, &pb.StatusConversionRequest{})
	upgradeConvertPrimariesStatus := &pb.UpgradeStepStatus{
		Step: pb.UpgradeSteps_CONVERT_PRIMARIES,
	}

	reconfigurePortsPath := filepath.Join(h.conf.StateDir, "reconfigure-ports")
	reconfigurePortsState := upgradestatus.NewStateCheck(reconfigurePortsPath, pb.UpgradeSteps_RECONFIGURE_PORTS)
	reconfigurePortsStatus, _ := reconfigurePortsState.GetStatus()

	statuses := strings.Join(conversionStatus.GetConversionStatuses(), " ")
	if strings.Contains(statuses, "FAILED") {
		upgradeConvertPrimariesStatus.Status = pb.StepStatus_FAILED
	} else if strings.Contains(statuses, "RUNNING") {
		upgradeConvertPrimariesStatus.Status = pb.StepStatus_RUNNING
	} else if strings.Contains(statuses, "COMPLETE") {
		upgradeConvertPrimariesStatus.Status = pb.StepStatus_COMPLETE
	} else {
		upgradeConvertPrimariesStatus.Status = pb.StepStatus_PENDING
	}

	return &pb.StatusUpgradeReply{
		ListOfUpgradeStepStatuses: []*pb.UpgradeStepStatus{
			checkconfigStatus,
			seginstallStatus,
			prepareInitStatus,
			shutdownClustersStatus,
			masterUpgradeStatus,
			startAgentsStatus,
			shareOidsStatus,
			validateStartClusterStatus,
			upgradeConvertPrimariesStatus,
			reconfigurePortsStatus,
		},
	}, nil
}

func (h *Hub) GetPrepareNewClusterConfigStatus() (*pb.UpgradeStepStatus, error) {
	/* Treat all stat failures as cannot find file. Conceal worse failures atm.*/
	_, err := utils.System.Stat(GetNewConfigFilePath(h.conf.StateDir))

	if err != nil {
		gplog.Debug("%v", err)
		return &pb.UpgradeStepStatus{
			Step:   pb.UpgradeSteps_PREPARE_INIT_CLUSTER,
			Status: pb.StepStatus_PENDING,
		}, nil
	}

	return &pb.UpgradeStepStatus{
		Step:   pb.UpgradeSteps_PREPARE_INIT_CLUSTER,
		Status: pb.StepStatus_COMPLETE,
	}, nil
}
