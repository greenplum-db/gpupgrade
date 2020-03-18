package agent

import (
	"context"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

var postgresFiles = [...]string {"postgresql.conf", "PG_VERSION"}

func (s *Server) DeleteDirectories(ctx context.Context, in *idl.DeleteDirectoriesRequest) (*idl.DeleteDirectoriesReply, error) {
	gplog.Info("got a request to delete data directories from the hub")

	var mErr *multierror.Error

	for _, segDataDir := range in.Datadirs {

		var postgresFilesError *multierror.Error

		for _, fileName := range postgresFiles {
			filePath := filepath.Join(segDataDir, fileName)
			_, err := utils.System.Stat(filePath)
			if err != nil {
				postgresFilesError = multierror.Append(postgresFilesError, err)
			}
		}

		if postgresFilesError != nil {
			mErr = multierror.Append(mErr, postgresFilesError.ErrorOrNil())
			continue
		}

		err := utils.System.RemoveAll(segDataDir)
		if err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	return &idl.DeleteDirectoriesReply{}, mErr.ErrorOrNil()
}
