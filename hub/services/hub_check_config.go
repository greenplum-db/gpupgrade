package services

import (
	"encoding/json"
	"fmt"
	"io"
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

var CONFIGQUERY5 = `SELECT
	s.content,
	s.hostname,
	e.fselocation as datadir,
	s.dbid,
	s.preferred_role,
	s.role,
	s.port
	FROM gp_segment_configuration s
	JOIN pg_filespace_entry e ON s.dbid = e.fsedbid
	JOIN pg_filespace f ON e.fsefsoid = f.oid
	WHERE s.role = 'p' AND f.fsname = 'pg_system'
	ORDER BY s.content;`

var CONFIGQUERY6 = `SELECT
	content,
	hostname,
	datadir,
	dbid,
	preferred_role,
	role,
	port
	FROM gp_segment_configuration
	WHERE role = 'p'
	ORDER BY content;`

func (h *HubClient) CheckConfig(ctx context.Context,
	in *pb.CheckConfigRequest) (*pb.CheckConfigReply, error) {
	gplog.Info("starting CheckConfig()")

	dbConnector := db.NewDBConn("localhost", int(in.DbPort), "template1")
	defer dbConnector.Close()
	err := dbConnector.Connect(1)
	if err != nil {
		gplog.Error(err.Error())
		return &pb.CheckConfigReply{}, utils.DatabaseConnectionError{Parent: err}
	}
	dbConnector.Version.Initialize(dbConnector)

	err = SaveOldClusterConfig(dbConnector, h.conf.StateDir, in.OldBinDir)
	if err != nil {
		gplog.Error(err.Error())
		return &pb.CheckConfigReply{}, err
	}

	successReply := &pb.CheckConfigReply{ConfigStatus: "All good"}

	return successReply, nil
}

func SaveOldClusterConfig(dbConnector *dbconn.DBConn, stateDir string, oldBinDir string) error {
	err := os.MkdirAll(stateDir, 0700)
	if err != nil {
		return err
	}

	configQuery := CONFIGQUERY6
	if dbConnector.Version.Before("6") {
		configQuery = CONFIGQUERY5
	}

	configFile := configutils.GetConfigFilePath(stateDir)
	configFileHandle, err := operating.System.OpenFileWrite(configFile, os.O_CREATE|os.O_WRONLY, 0700)
	if err != nil {
		errMsg := fmt.Sprintf("Unable to write to config file %s. Err: %s", configFile, err.Error())
		return errors.New(errMsg)
	}

	segConfig := make(configutils.SegmentConfiguration, 0)
	err = dbConnector.Select(&segConfig, configQuery)
	if err != nil {
		errMsg := fmt.Sprintf("Unable to execute query %s. Err: %s", configQuery, err.Error())
		return errors.New(errMsg)
	}

	configJSON := &configutils.ClusterConfig{
		SegConfig: segConfig,
		BinDir:    oldBinDir,
	}

	err = SaveQueryResultToJSON(configJSON, configFileHandle)
	if err != nil {
		return err
	}

	return nil
}

// public for testing purposes
func SaveQueryResultToJSON(structure interface{}, fileHandle io.WriteCloser) error {
	byteArr, err := json.Marshal(structure)
	if err != nil {
		errMsg := fmt.Sprintf("Unable to marshal query results to JSON. Err: %s", err.Error())
		return errors.New(errMsg)
	}

	_, err = fileHandle.Write(byteArr)
	if err != nil {
		errMsg := fmt.Sprintf("Unable to write query results to file. Err: %s", err.Error())
		return errors.New(errMsg)
	}

	return nil
}
