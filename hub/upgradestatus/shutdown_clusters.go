package upgradestatus

import (
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

type ShutDownClusters struct {
	gpstopStatePath string
	executor        cluster.Executor
}

func NewShutDownClusters(gpstopStatePath string, executor cluster.Executor) ShutDownClusters {
	return ShutDownClusters{
		gpstopStatePath: gpstopStatePath,
		executor:        executor,
	}
}

/*
 assumptions here are:
	- gpstop will not fail without error before writing an inprogress file
	- when a new gpstop is started it deletes all *.done and *.inprogress files
*/
func (s *ShutDownClusters) GetStatus() *pb.UpgradeStepStatus {
	var shutdownClustersStatus *pb.UpgradeStepStatus
	gpstopStatePath := s.gpstopStatePath

	if _, err := utils.System.Stat(gpstopStatePath); utils.System.IsNotExist(err) {
		shutdownClustersStatus = &pb.UpgradeStepStatus{
			Step:   pb.UpgradeSteps_STOPPED_CLUSTER,
			Status: pb.StepStatus_PENDING,
		}
		return shutdownClustersStatus
	}

	/* There can be cases where gpstop is running but not as part of the pre-setup
	 * in which case, we shouldn't be detecting that as a running state.
	 * We only care if the inprogress file exists. We are relying on the hub to never go down
	 * for this state processing to work.
	 */
	if s.isGpstopRunning() && s.inProgressFilesExist(gpstopStatePath) {
		shutdownClustersStatus = &pb.UpgradeStepStatus{
			Step:   pb.UpgradeSteps_STOPPED_CLUSTER,
			Status: pb.StepStatus_RUNNING,
		}
		return shutdownClustersStatus
	}

	if !s.inProgressFilesExist(gpstopStatePath) && s.IsStopComplete(gpstopStatePath) {
		shutdownClustersStatus = &pb.UpgradeStepStatus{
			Step:   pb.UpgradeSteps_STOPPED_CLUSTER,
			Status: pb.StepStatus_COMPLETE,
		}
		return shutdownClustersStatus
	}

	shutdownClustersStatus = &pb.UpgradeStepStatus{
		Step:   pb.UpgradeSteps_STOPPED_CLUSTER,
		Status: pb.StepStatus_FAILED,
	}

	return shutdownClustersStatus
}

func (s *ShutDownClusters) isGpstopRunning() bool {
	//if pgrep doesnt find target, ExecCmdOutput will return empty byte array and err.Error()="exit status 1"
	pgUpgradePids, err := s.executor.ExecuteLocalCommand("pgrep -f gpstop")
	if err == nil && len(pgUpgradePids) != 0 {
		return true
	}
	return false
}

func (s *ShutDownClusters) inProgressFilesExist(gpstopStatePath string) bool {
	files, err := utils.System.FilePathGlob(gpstopStatePath + "/*/in.progress")
	if files == nil {
		return false
	}

	if err != nil {
		gplog.Error("err is: ", err)
		return false
	}

	return true
}

func (s *ShutDownClusters) IsStopComplete(gpstopStatePath string) bool {
	completeFiles, completeErr := utils.System.FilePathGlob(gpstopStatePath + "/*/completed")
	if completeFiles == nil {
		return false
	}

	if completeErr != nil {
		gplog.Error(completeErr.Error())
		return false
	}

	/* There should only be two completed files.
	 * One for gpstop.old and one for gpstop.new
	 */
	if len(completeFiles) == 2 {
		return true
	}

	return false
}
