package services

import (
	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/db"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (h *Hub) CheckConfig(ctx context.Context, _ *pb.CheckConfigRequest) (*pb.CheckConfigReply, error) {
	gplog.Info("starting CheckConfig()")

	c := upgradestatus.NewChecklistManager(h.conf.StateDir)
	step := c.GetStepWriter(upgradestatus.CONFIG)

	// TODO: bubble these errors up.
	err := step.ResetStateDir()
	if err != nil {
		gplog.Error("error from ResetStateDir " + err.Error())
	}
	err = step.MarkInProgress()
	if err != nil {
		gplog.Error("error from MarkInProgress " + err.Error())
	}

	err = RetrieveAndSaveSourceConfig(h.conf.StateDir, h.source)
	if err != nil {
		step.MarkFailed()
		gplog.Error(err.Error())
		return &pb.CheckConfigReply{}, err
	}

	successReply := &pb.CheckConfigReply{ConfigStatus: "All good"}
	step.MarkComplete()

	return successReply, nil
}

// RetrieveAndSaveSourceConfig() fills in the rest of the clusterPair.OldCluster by
// querying the database located at its host and port. The results will
// additionally be written to disk.
func RetrieveAndSaveSourceConfig(stateDir string, source *utils.Cluster) error {
	dbConnector := db.NewDBConn("localhost", 0, "template1")
	err := dbConnector.Connect(1)
	if err != nil {
		return utils.DatabaseConnectionError{Parent: err}
	}
	defer dbConnector.Close()

	dbConnector.Version.Initialize(dbConnector)

	segConfigs, err := cluster.GetSegmentConfiguration(dbConnector)
	if err != nil {
		return errors.Wrap(err, "Unable to get segment configuration for old cluster")
	}

	source.Cluster = cluster.NewCluster(segConfigs)
	return source.Commit()
}
