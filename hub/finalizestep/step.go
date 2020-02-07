package finalizestep

import (
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"

	hubStep "github.com/greenplum-db/gpupgrade/hub/step"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

func Run(stream idl.CliToHub_FinalizeServer, stateDir string, source *utils.Cluster, target *utils.Cluster) error {
	s, err := hubStep.Begin(stateDir, "finalize", stream)
	if err != nil {
		return err
	}

	defer func() {
		if ferr := s.Finish(); ferr != nil {
			err = multierror.Append(err, ferr).ErrorOrNil()
		}

		if err != nil {
			gplog.Error(fmt.Sprintf("finalize: %s", err))
		}
	}()

	s.Run(idl.Substep_RECONFIGURE_PORTS, func(stream step.OutStreams) error {
		return ReconfigurePorts(stream, source, target)
	})

	return s.Err()
}
