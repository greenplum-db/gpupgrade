package agent

import (
	"context"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (s *Server) RenameDirectories(ctx context.Context, in *idl.RenameDirectoriesRequest) (*idl.RenameDirectoriesReply, error) {
	gplog.Info("agent received request to rename segment data directories")

	for _, pair := range in.GetPairs() {
		err := utils.System.Rename(pair.Src, pair.Dst)
		if err != nil {
			return &idl.RenameDirectoriesReply{}, err
		}
	}

	return &idl.RenameDirectoriesReply{}, nil
}
