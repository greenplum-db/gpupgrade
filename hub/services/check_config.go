package services

import (
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/db"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (h *Hub) CheckConfig(ctx context.Context, _ *idl.CheckConfigRequest) (*idl.CheckConfigReply, error) {
	gplog.Info("starting %s", upgradestatus.CONFIG)

	step, err := h.InitializeStep(upgradestatus.CONFIG)
	if err != nil {
		gplog.Error(err.Error())
		return &idl.CheckConfigReply{}, err
	}

	dbConn := db.NewDBConn("localhost", 0, "template1")
	defer dbConn.Close()
	err = ReloadAndCommitCluster(h.source, dbConn)
	if err != nil {
		gplog.Error(err.Error())
		step.MarkFailed()
		return &idl.CheckConfigReply{}, err
	}

	step.MarkComplete()
	return &idl.CheckConfigReply{}, err
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
