package services

import (
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/db"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (h *Hub) CheckConfig(ctx context.Context, in *pb.CheckConfigRequest) (*pb.CheckConfigReply, error) {
	gplog.Info("starting CheckConfig()")

	c := upgradestatus.NewChecklistManager(h.conf.StateDir)
	checkConfigStep := "check-config"

	err := c.ResetStateDir(checkConfigStep)
	if err != nil {
		gplog.Error("error from ResetStateDir " + err.Error())
	}
	err = c.MarkInProgress(checkConfigStep)
	if err != nil {
		gplog.Error("error from MarkInProgress " + err.Error())
	}

	dbConnector := db.NewDBConn("localhost", int(in.DbPort), "template1")
	defer dbConnector.Close()
	err = dbConnector.Connect(1)
	if err != nil {
		c.MarkFailed(checkConfigStep)
		gplog.Error(err.Error())
		return &pb.CheckConfigReply{}, utils.DatabaseConnectionError{Parent: err}
	}
	dbConnector.Version.Initialize(dbConnector)

	err = SaveOldClusterConfig(h.clusterPair, dbConnector, h.conf.StateDir, in.OldBinDir)
	if err != nil {
		c.MarkFailed(checkConfigStep)
		gplog.Error(err.Error())
		return &pb.CheckConfigReply{}, err
	}

	successReply := &pb.CheckConfigReply{ConfigStatus: "All good"}
	c.MarkComplete(checkConfigStep)

	return successReply, nil
}

func SaveOldClusterConfig(clusterPair *utils.ClusterPair, dbConnector *dbconn.DBConn, stateDir string, oldBinDir string) error {
	segConfigs, err := cluster.GetSegmentConfiguration(dbConnector)
	if err != nil {
		errMsg := fmt.Sprintf("Unable to get segment configuration for old cluster: %s", err.Error())
		return errors.New(errMsg)
	}
	clusterPair.OldCluster = cluster.NewCluster(segConfigs)
	clusterPair.OldBinDir = oldBinDir

	err = clusterPair.WriteOldConfig(stateDir)
	return err
}
