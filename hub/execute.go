package hub

import (
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
)

const executeMasterBackupName = "upgraded-master.bak"

func (s *Server) Execute(request *idl.ExecuteRequest, stream idl.CliToHub_ExecuteServer) (err error) {
	upgradedMasterBackupDir := filepath.Join(s.StateDir, executeMasterBackupName)

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

	st.Run(idl.Substep_SHUTDOWN_SOURCE_CLUSTER, func(streams step.OutStreams) error {
		return StopCluster(streams, s.Source, true)
	})

	st.Run(idl.Substep_UPGRADE_MASTER, func(streams step.OutStreams) error {
		stateDir := s.StateDir
		return UpgradeMaster(s.Source, s.Target, stateDir, streams, false, s.UseLinkMode)
	})

	st.Run(idl.Substep_COPY_MASTER, func(streams step.OutStreams) error {
		return s.CopyMasterDataDir(streams, upgradedMasterBackupDir)
	})

	st.Run(idl.Substep_UPGRADE_PRIMARIES, func(_ step.OutStreams) error {
		agentConns, err := s.AgentConns()

		if err != nil {
			return errors.Wrap(err, "failed to connect to gpupgrade agent")
		}

		dataDirPair, err := s.GetDataDirPairs()

		if err != nil {
			return errors.Wrap(err, "failed to get old and new primary data directories")
		}

		return UpgradePrimaries(false, upgradedMasterBackupDir, agentConns, dataDirPair, s.Source, s.Target, s.UseLinkMode)
	})

	st.Run(idl.Substep_START_TARGET_CLUSTER, func(streams step.OutStreams) error {
		return StartCluster(streams, s.Target, false)
	})

	return st.Err()
}
