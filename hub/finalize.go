package hub

import (
	"fmt"

	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
)

func (s *Server) Finalize(_ *idl.FinalizeRequest, stream idl.CliToHub_FinalizeServer) (err error) {
	store, err := s.NewSubstepStateStore("finalize")

	return Finalize(s.StateDir, s.Source, s.Target, stream, store)
}

func Finalize(stateDir string, source *utils.Cluster, target *utils.Cluster, stream idl.CliToHub_FinalizeServer, substepStateStore step.Store) (err error) {
	st, err := BeginStep(stateDir, "finalize", stream, substepStateStore)
	if err != nil {
		return err
	}

	defer func() {
		if ferr := st.Finish(); ferr != nil {
			err = multierror.Append(err, ferr).ErrorOrNil()
		}

		if err != nil {
			gplog.Error(fmt.Sprintf("finalize: %s", err))
		}
	}()

	st.Run(idl.Substep_UPGRADE_STANDBY, func(streams step.OutStreams) error {
		greenplumRunner := &greenplumRunner{
			masterPort:          target.MasterPort(),
			masterDataDirectory: target.MasterDataDir(),
			binDir:              target.BinDir,
		}

		return UpgradeStandby(greenplumRunner, StandbyConfig{
			Port:          target.StandbyPort(),
			Hostname:      target.StandbyHostname(),
			DataDirectory: target.StandbyDataDirectory() + "_upgrade",
		})
	})

	st.Run(idl.Substep_RECONFIGURE_PORTS, func(stream step.OutStreams) error {
		return ReconfigurePorts(source, target, stream)
	})

	return st.Err()
}
