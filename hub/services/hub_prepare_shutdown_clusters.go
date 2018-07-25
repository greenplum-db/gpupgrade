package services

import (
	"fmt"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"golang.org/x/net/context"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

func (h *Hub) PrepareShutdownClusters(ctx context.Context, in *pb.PrepareShutdownClustersRequest) (*pb.PrepareShutdownClustersReply, error) {
	gplog.Info("starting PrepareShutdownClusters()")

	go h.ShutdownClusters()

	return &pb.PrepareShutdownClustersReply{}, nil
}

func (h *Hub) ShutdownClusters() {
	step := h.checklist.GetStepWriter(upgradestatus.SHUTDOWN_CLUSTERS)

	step.ResetStateDir()
	step.MarkInProgress()

	var errSource error
	errSource = StopCluster(h.source)
	if errSource != nil {
		gplog.Error(errSource.Error())
	}

	var errTarget error
	errTarget = StopCluster(h.target)
	if errTarget != nil {
		gplog.Error(errTarget.Error())
	}

	if errSource != nil || errTarget != nil {
		step.MarkFailed()
		return
	}

	step.MarkComplete()
}

func StopCluster(c *utils.Cluster) error {
	if !IsPostmasterRunning(c) {
		return nil
	}

	masterDataDir := c.MasterDataDir()
	gpstopShellArgs := fmt.Sprintf("source %[1]s/../greenplum_path.sh; %[1]s/gpstop -a -d %[2]s", c.BinDir, masterDataDir)

	gplog.Info("gpstop args: %+v", gpstopShellArgs)
	_, err := c.ExecuteLocalCommand(gpstopShellArgs)
	if err != nil {
		return err
	}

	return nil
}

func IsPostmasterRunning(c *utils.Cluster) bool {
	masterDataDir := c.MasterDataDir()
	checkPidCmd := fmt.Sprintf("pgrep -F %s/postmaster.pid", masterDataDir)

	_, err := c.ExecuteLocalCommand(checkPidCmd)
	if err != nil {
		gplog.Error("Could not determine whether the cluster with MASTER_DATA_DIRECTORY: %s is running: %+v",
			masterDataDir, err)
		return false
	}

	return true
}
