package services

import (
	"fmt"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"golang.org/x/net/context"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"github.com/hashicorp/go-multierror"
	"github.com/greenplum-db/gpupgrade/utils/log"
)

func (h *Hub) PrepareShutdownClusters(ctx context.Context, in *idl.PrepareShutdownClustersRequest) (*idl.PrepareShutdownClustersReply, error) {
	gplog.Info("starting %s", upgradestatus.SHUTDOWN_CLUSTERS)

	step, err := h.InitializeStep(upgradestatus.SHUTDOWN_CLUSTERS)
	if err != nil {
		gplog.Error(err.Error())
		return &idl.PrepareShutdownClustersReply{}, err
	}

	go func() {
		defer log.WritePanics()

		if err := h.ShutdownClusters(); err != nil {
			gplog.Error(err.Error())
			step.MarkFailed()
		} else {
			step.MarkComplete()
		}
	}()

	return &idl.PrepareShutdownClustersReply{}, nil
}

func (h *Hub) ShutdownClusters() error {
	var shutdownErr error

	err := StopCluster(h.source)
	if err != nil {
		shutdownErr = multierror.Append(shutdownErr, errors.Wrap(err, "failed to stop source cluster"))
	}

	err = StopCluster(h.target)
	if err != nil {
		shutdownErr = multierror.Append(shutdownErr, errors.Wrap(err, "failed to stop target cluster"))
	}

	return shutdownErr
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
