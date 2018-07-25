package services

import (
	"github.com/greenplum-db/gpupgrade/db"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/net/context"
)

const (
	MINIMUM_VERSION = "4.3.9"
)

func (h *Hub) CheckVersion(ctx context.Context,
	in *pb.CheckVersionRequest) (*pb.CheckVersionReply, error) {

	gplog.Info("starting CheckVersion")

	masterPort := h.source.MasterPort()

	dbConnector := db.NewDBConn("localhost", masterPort, "template1")
	defer dbConnector.Close()
	err := dbConnector.Connect(1)
	if err != nil {
		gplog.Error(err.Error())
		return &pb.CheckVersionReply{}, utils.DatabaseConnectionError{Parent: err}
	}
	dbConnector.Version.Initialize(dbConnector)

	isVersionCompatible := dbConnector.Version.AtLeast(MINIMUM_VERSION)
	return &pb.CheckVersionReply{IsVersionCompatible: isVersionCompatible}, nil
}
