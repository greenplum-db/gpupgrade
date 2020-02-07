package initializestep

import (
	"context"
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/hub/agent"
	hubStep "github.com/greenplum-db/gpupgrade/hub/step"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

func Run(stream idl.CliToHub_InitializeServer, in *idl.InitializeRequest, stateDir string, agentPort int, saveConfig func(source *utils.Cluster, target *utils.Cluster) error) error {
	st, err := hubStep.Begin(stateDir, "initialize", stream)
	if err != nil {
		return err
	}

	var source *utils.Cluster

	defer func() {
		if ferr := st.Finish(); ferr != nil {
			err = multierror.Append(err, ferr).ErrorOrNil()
		}

		if err != nil {
			gplog.Error(fmt.Sprintf("initialize: %s", err))
		}
	}()

	st.Run(idl.Substep_CONFIG, func(stream step.OutStreams) error {
		source, _, err = FillClusterConfigsSubStep(stream, in, saveConfig)
		return err
	})

	st.Run(idl.Substep_START_AGENTS, func(_ step.OutStreams) error {
		_, err := agent.RestartAll(context.Background(), nil, source.GetHostnames(), agentPort, stateDir)
		return err
	})

	return st.Err()
}
