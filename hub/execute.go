package hub

import (
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
)

func (s *Server) Execute(request *idl.ExecuteRequest, stream idl.CliToHub_ExecuteServer) (err error) {
	st, err := BeginStep(s.StateDir, "execute", stream)
	if err != nil {
		return err
	}

	defer func() {
		if ferr := st.Finish(); ferr != nil {
			err = multierror.Append(err, ferr).ErrorOrNil()
		}

		if err != nil {
			gplog.Error(fmt.Sprintf("execute: %s", err))
		}
	}()

	st.Run(idl.Substep_SHUTDOWN_SOURCE_CLUSTER, func(stream step.OutStreams) error {
		return StopCluster(stream, s.Source)
	})

	st.Run(idl.Substep_UPGRADE_MASTER, func(streams step.OutStreams) error {
		stateDir := s.StateDir
		return UpgradeMaster(s.Source, s.Target, stateDir, streams, false, s.UseLinkMode)
	})

	st.Run(idl.Substep_COPY_MASTER, s.CopyMasterDataDir)

	st.Run(idl.Substep_UPGRADE_PRIMARIES, func(_ step.OutStreams) error {
		return s.ConvertPrimaries(false)
	})

	st.Run(idl.Substep_START_TARGET_CLUSTER, func(streams step.OutStreams) error {
		return StartCluster(streams, s.Target)
	})

	return st.Err()
}
