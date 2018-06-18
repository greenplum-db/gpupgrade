package services

import (
	"fmt"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"

	"golang.org/x/net/context"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

func (h *Hub) PrepareShutdownClusters(ctx context.Context, in *pb.PrepareShutdownClustersRequest) (*pb.PrepareShutdownClustersReply, error) {
	gplog.Info("starting PrepareShutdownClusters()")

	go h.ShutdownClusters()

	return &pb.PrepareShutdownClustersReply{}, nil
}

func (h *Hub) ShutdownClusters() {
	step := upgradestatus.SHUTDOWN_CLUSTERS

	h.checklistWriter.ResetStateDir(step)
	h.checklistWriter.MarkInProgress(step)

	var errOld error
	errOld = StopCluster(h.clusterPair.OldCluster, h.clusterPair.OldBinDir)
	if errOld != nil {
		gplog.Error(errOld.Error())
	}

	var errNew error
	errNew = StopCluster(h.clusterPair.NewCluster, h.clusterPair.NewBinDir)
	if errNew != nil {
		gplog.Error(errNew.Error())
	}

	if errOld != nil || errNew != nil {
		h.checklistWriter.MarkFailed(step)
		return
	}

	h.checklistWriter.MarkComplete(step)
}

func StopCluster(c *cluster.Cluster, binDir string) error {
	if !IsPostmasterRunning(c) {
		return nil
	}

	masterDataDir := c.GetDirForContent(-1)
	gpstopShellArgs := fmt.Sprintf("source %[1]s/../greenplum_path.sh; %[1]s/gpstop -a -d %[2]s", binDir, masterDataDir)

	gplog.Info("gpstop args: %+v", gpstopShellArgs)
	_, err := c.ExecuteLocalCommand(gpstopShellArgs)
	if err != nil {
		return err
	}

	return nil
}

func IsPostmasterRunning(c *cluster.Cluster) bool {
	masterDataDir := c.GetDirForContent(-1)
	checkPidCmd := fmt.Sprintf("pgrep -F %s/postmaster.pid", masterDataDir)

	_, err := c.ExecuteLocalCommand(checkPidCmd)
	if err != nil {
		gplog.Error("Could not determine whether the cluster with MASTER_DATA_DIRECTORY: %s is running: %+v",
			masterDataDir, err)
		return false
	}

	return true
}
