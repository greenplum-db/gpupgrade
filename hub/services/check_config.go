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
	"github.com/greenplum-db/gpupgrade/utils/log"
)

func (h *Hub) CheckConfig(ctx context.Context, _ *idl.CheckConfigRequest) (*idl.CheckConfigReply, error) {
	gplog.Info("starting %s", upgradestatus.CONFIG)
	defer log.WritePanics()

	stepWriter, err := h.WriteStep(upgradestatus.CONFIG)
	if err != nil {
		gplog.Error(err.Error())
		return &idl.CheckConfigReply{}, err
	}

	dbConn := db.NewDBConn("localhost", 0, "template1")
	defer dbConn.Close()
	err = ReloadAndCommitCluster(h.source, dbConn)
	if err != nil {
		gplog.Error(err.Error())
		stepWriter.MarkFailed()
		return &idl.CheckConfigReply{}, err
	}

	stepWriter.MarkComplete()
	return &idl.CheckConfigReply{ConfigStatus: "success"}, nil
}

// ReloadAndCommitCluster() will fill in a utils.Cluster using a database
// connection and additionally write the results to disk.
func ReloadAndCommitCluster(cluster *utils.Cluster, conn *dbconn.DBConn) error {
	newCluster, err := utils.ClusterFromDB(conn, cluster.BinDir, cluster.ConfigPath)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve cluster configuration")
	}

	*cluster = *newCluster
	err = cluster.Commit()
	if err != nil {
		return errors.Wrap(err, "failed to save cluster configuration")
	}

	return nil
}
