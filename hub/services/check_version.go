package services

import (
	"github.com/greenplum-db/gpupgrade/db"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/net/context"
)

// FIXME: we need to rework this as a check for:
//           minimum source gpdb version (e.g. at least 5.15)
//           minimum/maximum target gpdb version (e.g. at least 6.2 but less than 7.0)
//        also, return the actual source/target gpdb versions here

const (
	MINIMUM_VERSION = "5.0.0" // FIXME: set to minimum 5.X version we support
)

func (h *Hub) CheckVersion(ctx context.Context,
	in *idl.CheckVersionRequest) (*idl.CheckVersionReply, error) {

	gplog.Info("starting CheckVersion")

	masterPort := h.source.MasterPort()

	dbConnector := db.NewDBConn("localhost", masterPort, "template1")
	defer dbConnector.Close()
	err := dbConnector.Connect(1)
	if err != nil {
		gplog.Error(err.Error())
		return &idl.CheckVersionReply{}, utils.DatabaseConnectionError{Parent: err}
	}

	isVersionCompatible := dbConnector.Version.AtLeast(MINIMUM_VERSION)
	return &idl.CheckVersionReply{IsVersionCompatible: isVersionCompatible}, nil
}
