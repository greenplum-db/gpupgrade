package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

// grpc generated function signature requires ctx and in params.
// nolint: unparam
func (h *Hub) CheckSeginstall(ctx context.Context, in *idl.CheckSeginstallRequest) (*idl.CheckSeginstallReply, error) {
	gplog.Info("Running CheckSeginstall()")

	err := h.checklistWriter.ResetStateDir(upgradestatus.SEGINSTALL)
	if err != nil {
		gplog.Error(err.Error())
		return &idl.CheckSeginstallReply{}, err
	}

	err = h.checklistWriter.MarkInProgress(upgradestatus.SEGINSTALL)
	if err != nil {
		gplog.Error(err.Error())
		return &idl.CheckSeginstallReply{}, err
	}

	go VerifyAgentsInstalled(h.clusterPair, h.checklistWriter)

	return &idl.CheckSeginstallReply{}, nil
}

func VerifyAgentsInstalled(cp *utils.ClusterPair, cw ChecklistWriter) {
	var err error

	// TODO: if this finds nothing, should we err out? do a fallback check based on $GPHOME?
	logStr := "check gpupgrade_agent is installed in GPHOME on master and hosts"
	agentPath := filepath.Join(os.Getenv("GPHOME"), "bin", "gpupgrade_agent")
	returnLsCommand := func(contentID int) string { return "ls " + agentPath }
	remoteOutput := cp.OldCluster.GenerateAndExecuteCommand(logStr, returnLsCommand, cluster.ON_HOSTS_AND_MASTER)

	errStr := "Failed to find all gpupgrade_agents"
	errMessage := func(contentID int) string {
		return fmt.Sprintf("Could not find gpupgrade_agent on segment with contentID %d", contentID)
	}
	cp.OldCluster.CheckClusterError(remoteOutput, errStr, errMessage, true)

	if remoteOutput.NumErrors > 0 {
		err = cw.MarkFailed(upgradestatus.SEGINSTALL)
		if err != nil {
			gplog.Error(err.Error())
			return
		}
	}

	err = cw.MarkComplete(upgradestatus.SEGINSTALL)
	if err != nil {
		gplog.Error(err.Error())
		return
	}
}
