package services

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/db"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (h *Hub) Initialize(ctx context.Context, in *idl.InitializeRequest) (*idl.InitializeReply, error) {
	err := h.fillClusterConfigsSubStep(in.OldBinDir, in.NewBinDir, int(in.OldPort))
	if err != nil {
		return &idl.InitializeReply{}, err
	}

	err = h.startAgentsSubStep()
	if err != nil {
		return &idl.InitializeReply{}, err
	}

	return &idl.InitializeReply{}, nil
}

// create old/new clusters, write to disk and re-read from disk to make sure it is "durable"
func (h *Hub) fillClusterConfigsSubStep(oldBinDir, newBinDir string, oldPort int) error {
	gplog.Info("starting %s", upgradestatus.CONFIG)

	step, err := h.InitializeStep(upgradestatus.CONFIG)
	if err != nil {
		gplog.Error(err.Error())
		return err
	}

	err = h.fillClusterConfigs(oldBinDir, newBinDir, oldPort)

	if err != nil {
		gplog.Error(err.Error())
		step.MarkFailed()
		return err
	}

	step.MarkComplete()
	return nil
}

func (h *Hub) startAgentsSubStep() error {
	gplog.Info("starting %s", upgradestatus.START_AGENTS)

	step, err := h.InitializeStep(upgradestatus.START_AGENTS)
	if err != nil {
		gplog.Error(err.Error())
		return err
	}

	err = StartAgents(h.source, h.target)
	if err != nil {
		gplog.Error(err.Error())
		step.MarkFailed()
		return err
	}

	step.MarkComplete()
	return nil
}

func (h *Hub) fillClusterConfigs(oldBinDir, newBinDir string, oldPort int) error {
	source := &utils.Cluster{BinDir: path.Clean(oldBinDir), ConfigPath: filepath.Join(h.conf.StateDir, utils.SOURCE_CONFIG_FILENAME)}
	dbConn := db.NewDBConn("localhost", oldPort, "template1")
	defer dbConn.Close()
	err := ReloadAndCommitCluster(source, dbConn)
	if err != nil {
		return err
	}

	emptyCluster := cluster.NewCluster([]cluster.SegConfig{})
	target := &utils.Cluster{Cluster: emptyCluster, BinDir: path.Clean(newBinDir), ConfigPath: filepath.Join(h.conf.StateDir, utils.TARGET_CONFIG_FILENAME)}
	err = target.Commit()
	if err != nil {
		return errors.Wrap(err, "Unable to save target cluster configuration")
	}

	// XXX: This is really not necessary as we are just verifying that
	// the configuration that we just wrote is readable.
	errSource := source.Load()
	errTarget := target.Load()
	if errSource != nil && errTarget != nil {
		errBoth := errors.Errorf("Source error: %s\nTarget error: %s", errSource.Error(), errTarget.Error())
		return errors.Wrap(errBoth, "Unable to load source or target cluster configuration")
	} else if errSource != nil {
		return errors.Wrap(errSource, "Unable to load source cluster configuration")
	} else if errTarget != nil {
		return errors.Wrap(errTarget, "Unable to load target cluster configuration")
	}

	// link in source/target to hub
	h.source = source
	h.target = target

	return err
}

// ReloadAndCommitCluster() will fill in a utils.Cluster using a database
// connection and additionally write the results to disk.
func ReloadAndCommitCluster(cluster *utils.Cluster, conn *dbconn.DBConn) error {
	newCluster, err := utils.ClusterFromDB(conn, cluster.BinDir, cluster.ConfigPath)
	if err != nil {
		return errors.Wrap(err, "could not retrieve cluster configuration")
	}

	*cluster = *newCluster
	err = cluster.Commit()
	if err != nil {
		return errors.Wrap(err, "could not save cluster configuration")
	}

	return nil
}

func StartAgents(source *utils.Cluster, target *utils.Cluster) error {
	// XXX If there are failures, does it matter what agents have successfully
	// started, or do we just want to stop all of them and kick back to the
	// user?
	logStr := "start agents on master and hosts"
	agentPath := filepath.Join(target.BinDir, "gpupgrade_agent")
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

	// Agents print their port and PID to stdout; log them for posterity.
	for content, output := range remoteOutput.Stdouts {
		if remoteOutput.Errors[content] == nil {
			gplog.Info("[%s] %s", source.Segments[content].Hostname, output)
		}
	}

	if remoteOutput.NumErrors > 0 {
		// CheckClusterError() will have already logged each error.
		return errors.New("could not start agents on segment hosts; see log for details")
	}

	return nil
}
