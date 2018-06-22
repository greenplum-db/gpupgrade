package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/hub/cluster_ssher"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
)

// grpc generated function signature requires ctx and in params.
// nolint: unparam
func (h *Hub) PrepareStartAgents(ctx context.Context, in *idl.PrepareStartAgentsRequest) (*idl.PrepareStartAgentsReply, error) {
	gplog.Info("Running PrepareStartAgents()")

	err := h.checklistWriter.ResetStateDir(upgradestatus.START_AGENTS)
	if err != nil {
		gplog.Error(err.Error())
		return &idl.PrepareStartAgentsReply{}, err
	}

	err = h.checklistWriter.MarkInProgress(upgradestatus.START_AGENTS)
	if err != nil {
		gplog.Error(err.Error())
		return &idl.PrepareStartAgentsReply{}, err
	}

	go StartAgents(h.clusterPair, h.checklistWriter)

	return &idl.PrepareStartAgentsReply{}, nil
}

func StartAgents(cp *ClusterPair, cw cluster_ssher.ChecklistWriter) {
	var err error

	// TODO: if this finds nothing, should we err out? do a fallback check based on $GPHOME?
	logStr := "start agents on master and hosts"
	agentPath := filepath.Join(os.Getenv("GPHOME"), "bin", "gpupgrade_agent")
	runAgentCmd := func(contentID int) string { return agentPath + " --daemonize" }
	remoteOutput := cp.OldCluster.GenerateAndExecuteCommand(logStr, runAgentCmd, cluster.ON_HOSTS_AND_MASTER)

	errStr := "Failed to start all gpupgrade_agents"
	errMessage := func(contentID int) string {
		return fmt.Sprintf("Could not start gpupgrade_agent on segment with contentID %d", contentID)
	}
	cp.OldCluster.CheckClusterError(remoteOutput, errStr, errMessage, true)

	if remoteOutput.NumErrors > 0 {
		err = cw.MarkFailed(upgradestatus.START_AGENTS)
		if err != nil {
			gplog.Error(err.Error())
			return
		}
	}

	err = cw.MarkComplete(upgradestatus.START_AGENTS)
	if err != nil {
		gplog.Error(err.Error())
		return
	}
}
