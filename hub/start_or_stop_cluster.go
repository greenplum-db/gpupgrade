package hub

import (
	"github.com/greenplum-db/gpupgrade/hub/cluster"

	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/step"
)

func (s *Server) ShutdownCluster(stream step.OutStreams, isSource bool) error {
	if isSource {
		err := cluster.StopCluster(stream, s.Source)
		if err != nil {
			return errors.Wrap(err, "failed to stop source cluster")
		}
	} else {
		err := cluster.StopCluster(stream, s.Target)
		if err != nil {
			return errors.Wrap(err, "failed to stop target cluster")
		}
	}

	return nil
}
