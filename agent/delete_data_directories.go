package agent

import (
	"context"
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (s *Server) DeleteDirectories(ctx context.Context, in *idl.DeleteDirectoriesRequest) (*idl.DeleteDirectoriesReply, error) {
	gplog.Info("got a request to delete data directories from the hub")

	var mErr *multierror.Error

	for _, segDataDir := range in.Datadirs {

		// to allow idempotence...if the segDataDir is gone, it has already been deleted
		if !utils.DoesPathExist(segDataDir) {
			continue
		}

		if !IsPostgres(segDataDir) {
			mErr = multierror.Append(mErr, fmt.Errorf("could not delete non-postgres data dir: %s", segDataDir))
			continue
		}

		err := utils.System.RemoveAll(segDataDir)
		if err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	return &idl.DeleteDirectoriesReply{}, mErr.ErrorOrNil()
}
