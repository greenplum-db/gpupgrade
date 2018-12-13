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

func (h *Hub) CheckSeginstall(ctx context.Context, in *idl.CheckSeginstallRequest) (*idl.CheckSeginstallReply, error) {
	gplog.Info("starting %s", upgradestatus.SEGINSTALL)

	step, err := h.InitializeStep(upgradestatus.SEGINSTALL)
	if err != nil {
		gplog.Error(err.Error())
		return &idl.CheckSeginstallReply{}, err
	}

	go func() {
		defer log.WritePanics()

		if err := VerifyAgentsInstalled(h.source); err != nil {
			gplog.Error(err.Error())
			step.MarkFailed()
		} else {
			step.MarkComplete()
		}
	}()

	return &idl.CheckSeginstallReply{}, nil
}

func VerifyAgentsInstalled(source *utils.Cluster) error {
	logStr := "check gpupgrade_agent is installed in cluster's binary directory on master and hosts"
	agentPath := filepath.Join(source.BinDir, "gpupgrade_agent")
	returnLsCommand := func(contentID int) string { return "ls " + agentPath }

	remoteOutput, err := source.ExecuteOnAllHosts(logStr, returnLsCommand)
	if err != nil {
		return errors.Wrap(err, "could not verify agent installation")
	}

	errStr := "Failed to find all gpupgrade_agents"
	errMessage := func(contentID int) string {
		return fmt.Sprintf("Could not find gpupgrade_agent on segment with contentID %d", contentID)
	}
	source.CheckClusterError(remoteOutput, errStr, errMessage, true)

	if remoteOutput.NumErrors > 0 {
		// CheckClusterError() will have already logged each error.
		return errors.New("gpupgrade_agent is not installed on every segment; see log for details")
	}

	return nil
}
