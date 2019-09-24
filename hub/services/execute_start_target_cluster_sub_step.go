package services

import (
	"fmt"
	"io/ioutil"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (h *Hub) ExecuteStartTargetClusterSubStep(stream idl.CliToHub_ExecuteServer) error {
	gplog.Info("starting %s", upgradestatus.VALIDATE_START_CLUSTER)

	step, err := h.InitializeStep(upgradestatus.VALIDATE_START_CLUSTER)
	if err != nil {
		gplog.Error(err.Error())
		return err
	}

	err = startNewCluster(stream, h.target)
	if err != nil {
		gplog.Error(err.Error())
		step.MarkFailed()
	} else {
		step.MarkComplete()
	}

	return nil
}

func startNewCluster(stream idl.CliToHub_ExecuteServer, targetCluster *utils.Cluster) error {
	cmd := execCommand("bash", "-c",
		fmt.Sprintf("source %s/../greenplum_path.sh && %s/gpstart -a -d %s",
			targetCluster.BinDir,
			targetCluster.BinDir,
			targetCluster.MasterDataDir(),
		))

	mux := newMultiplexedStream(stream, ioutil.Discard)
	cmd.Stdout = mux.NewStreamWriter(idl.Chunk_STDOUT)
	cmd.Stderr = mux.NewStreamWriter(idl.Chunk_STDERR)

	return cmd.Run()
}
