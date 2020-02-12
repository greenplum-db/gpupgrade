package agent

import (
	"context"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (s *Server) ReconfigureDataDirectories(ctx context.Context, in *idl.ReconfigureDataDirRequest) (*idl.ReconfigureDataDirReply, error) {
	gplog.Info("agent received a request to move segment data directories from the hub")

	err := ReconfigureDataDirectories(in.Pairs)

	return &idl.ReconfigureDataDirReply{}, err
}

func ReconfigureDataDirectories(renamePairs []*idl.RenamePair) error {
	for _, pair := range renamePairs {
		gplog.Info("agent is moving %v to %v", pair.Src, pair.Dst)

		err := utils.System.Rename(pair.Src, pair.Dst)

		if err != nil {
			return err
		}
	}

	return nil
}
