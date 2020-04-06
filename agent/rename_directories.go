package agent

import (
	"context"
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

// The right-hand side functions are tested elsewhere, so to test RenameDataDirectories
//   we only need to spy on the calls made in this function.  We enable this via global
//   function injection.
var (
	IsPostgresFunc     = IsPostgres         //func IsPostgres(dataDir string) error
	RenameDataDirsFunc = hub.RenameDataDirs //func RenameDataDirs(source, target string, upgradeID upgrade.ID) error
)

func (s *Server) RenameDataDirectories(ctx context.Context, in *idl.RenameDataDirectoriesRequest) (
	*idl.RenameDataDirectoriesReply, error) {

	gplog.Info("agent received request to rename segment data directories")

	var mErr *multierror.Error

	for _, dirs := range in.GetDataDirs() {

		// the idempotence check is done in RenameDataDirsFunc.  However, since this agent can be called with
		//   any args, we make sure that we are called with proper arguments.  A non-existent dir path might
		//   be ok for idempotence, hence we accept that here and defer to RenameDataDirsFunc().
		if utils.DoesPathExist(dirs.Source) && !IsPostgresFunc(dirs.Source) {
			mErr = multierror.Append(mErr, fmt.Errorf("could not delete non-postgres data dir: %s", dirs.Source))
			continue
		}
		if utils.DoesPathExist(dirs.Target) && !IsPostgresFunc(dirs.Target) {
			mErr = multierror.Append(mErr, fmt.Errorf("could not delete non-postgres data dir: %s", dirs.Target))
			continue
		}

		if err := RenameDataDirsFunc(dirs.Source, dirs.Target, upgrade.ID(in.UpgradeID)); err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	return &idl.RenameDataDirectoriesReply{}, mErr.ErrorOrNil()
}
