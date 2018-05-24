package services

import (
	"fmt"
	"os"

	"github.com/greenplum-db/gpupgrade/db"
	"github.com/greenplum-db/gpupgrade/hub/configutils"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gp-common-go-libs/operating"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func SaveTargetClusterConfig(dbConnector *dbconn.DBConn, stateDir string, newBinDir string) error {
	segConfig := make(configutils.SegmentConfiguration, 0)
	var configQuery string

	configQuery = CONFIGQUERY6
	if dbConnector.Version.Before("6") {
		configQuery = CONFIGQUERY5
	}

	configFile, err := operating.System.OpenFileWrite(configutils.GetNewClusterConfigFilePath(stateDir), os.O_CREATE|os.O_WRONLY, 0700)
	if err != nil {
		errMsg := fmt.Sprintf("Could not open new config file for writing. Err: %s", err.Error())
		return errors.New(errMsg)
	}

	err = dbConnector.Select(&segConfig, configQuery)
	if err != nil {
		errMsg := fmt.Sprintf("Unable to execute query %s. Err: %s", configQuery, err.Error())
		return errors.New(errMsg)
	}

	clusterConfig := configutils.ClusterConfig{
		SegConfig: segConfig,
		BinDir:    newBinDir,
	}

	err = SaveQueryResultToJSON(&clusterConfig, configFile)
	if err != nil {
		return err
	}

	return nil
}

func (h *Hub) PrepareInitCluster(ctx context.Context, in *pb.PrepareInitClusterRequest) (*pb.PrepareInitClusterReply, error) {
	gplog.Info("starting PrepareInitCluster()")

	dbConnector := db.NewDBConn("localhost", int(in.DbPort), "template1")
	defer dbConnector.Close()
	err := dbConnector.Connect(1)
	if err != nil {
		gplog.Error(err.Error())
		return &pb.PrepareInitClusterReply{}, utils.DatabaseConnectionError{Parent: err}
	}
	dbConnector.Version.Initialize(dbConnector)

	err = SaveTargetClusterConfig(dbConnector, h.conf.StateDir, in.NewBinDir)
	if err != nil {
		gplog.Error(err.Error())
		return &pb.PrepareInitClusterReply{}, err
	}

	return &pb.PrepareInitClusterReply{}, nil
}
