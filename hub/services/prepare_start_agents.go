package services

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/log"
)

func (h *Hub) PrepareStartAgents(ctx context.Context, in *idl.PrepareStartAgentsRequest) (*idl.PrepareStartAgentsReply, error) {
	gplog.Info("starting %s", upgradestatus.START_AGENTS)

	step, err := h.InitializeStep(upgradestatus.START_AGENTS)
	if err != nil {
		gplog.Error(err.Error())
		return &idl.PrepareStartAgentsReply{}, err
	}

	go func() {
		defer log.WritePanics()

		if err := StartAgents(h.source); err != nil {
			gplog.Error(err.Error())
			step.MarkFailed()
		} else {
			step.MarkComplete()
		}
	}()

	return &idl.PrepareStartAgentsReply{}, nil
}

func StartAgents(source *utils.Cluster) error {
	logStr := "start agents on master and hosts"
	agentPath := filepath.Join(source.BinDir, "gpupgrade_agent")
	runAgentCmd := func(contentID int) string { return agentPath + " --daemonize" }

	errStr := "Failed to start all gpupgrade_agents"

	remoteOutput, err := source.ExecuteOnAllHosts(logStr, runAgentCmd)
	if err != nil {
		return errors.Wrap(err, errStr)
	}

	errMessage := func(contentID int) string {
		return fmt.Sprintf("Could not start gpupgrade_agent on segment with contentID %d", contentID)
	}
	source.CheckClusterError(remoteOutput, errStr, errMessage, true)

	if remoteOutput.NumErrors > 0 {
		// CheckClusterError() will have already logged each error.
		return errors.New("could not start agents on segment hosts; see log for details")
	}

	return nil
}
