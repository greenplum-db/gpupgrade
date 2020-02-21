package hub

import (
	"golang.org/x/net/context"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

// FIXME: we need to rework this as a check for:
//           minimum source gpdb version (e.g. at least 5.15)
//           minimum/maximum target gpdb version (e.g. at least 6.2 but less than 7.0)
//        also, return the actual source/target gpdb versions here

const (
	MINIMUM_VERSION = "5.0.0" // FIXME: set to minimum 5.X version we support
)

func (s *Server) CheckVersion(ctx context.Context,
	in *idl.CheckVersionRequest) (*idl.CheckVersionReply, error) {

	gplog.Info("starting CheckVersion")

	user, err := utils.GetUser()
	if err != nil {
		return &idl.CheckVersionReply{}, xerrors.Errorf("getting username: %w")
	}

	dbConnector := dbconn.NewDBConn("template1", user, "localhost", s.Source.MasterPort())
	defer dbConnector.Close()
	err = dbConnector.Connect(1)
	if err != nil {
		gplog.Error(err.Error())
		return &idl.CheckVersionReply{}, xerrors.Errorf("connecting to database %w", err)
	}

	isVersionCompatible := dbConnector.Version.AtLeast(MINIMUM_VERSION)
	return &idl.CheckVersionReply{IsVersionCompatible: isVersionCompatible}, nil
}
