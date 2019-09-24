package services

import (
	"fmt"
	"io/ioutil"
	"os/exec"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

var execCommandStopCluster = exec.Command
var execCommandIsPostmasterRunning = exec.Command

func (h *Hub) ExecuteShutdownClustersSubStep(stream idl.CliToHub_ExecuteServer) error {
	gplog.Info("starting %s", upgradestatus.SHUTDOWN_CLUSTERS)

	step, err := h.InitializeStep(upgradestatus.SHUTDOWN_CLUSTERS)
	if err != nil {
		gplog.Error(err.Error())
		return err
	}

	err = h.ShutdownClusters(stream)
	if err != nil {
		gplog.Error(err.Error())
		step.MarkFailed()
	} else {
		step.MarkComplete()
	}

	return err
}

func (h *Hub) ShutdownClusters(stream idl.CliToHub_ExecuteServer) error {
	var shutdownErr error

	err := StopCluster(stream, h.source)
	if err != nil {
		shutdownErr = multierror.Append(shutdownErr, errors.Wrap(err, "failed to stop old cluster"))
	}

	err = StopCluster(stream, h.target)
	if err != nil {
		shutdownErr = multierror.Append(shutdownErr, errors.Wrap(err, "failed to stop new cluster"))
	}

	return shutdownErr
}

func StopCluster(stream idl.CliToHub_ExecuteServer, c *utils.Cluster) error {
	err := IsPostmasterRunning(stream, c)
	if err != nil {
		return err
	}

	cmd := execCommandStopCluster("bash", "-c",
			fmt.Sprintf("source %[1]s/../greenplum_path.sh && %[1]s/gpstop -a -d %[2]s",
				c.BinDir,
				c.MasterDataDir(),
			))

	mux := newMultiplexedStream(stream, ioutil.Discard)
	cmd.Stdout = mux.NewStreamWriter(idl.Chunk_STDOUT)
	cmd.Stderr = mux.NewStreamWriter(idl.Chunk_STDERR)

	return cmd.Run()
}

func IsPostmasterRunning(stream idl.CliToHub_ExecuteServer, c *utils.Cluster) error {
	cmd := execCommandIsPostmasterRunning("bash", "-c",
		fmt.Sprintf("pgrep -F %s/postmaster.pid",
			c.MasterDataDir(),
		))

	mux := newMultiplexedStream(stream, ioutil.Discard)
	cmd.Stdout = mux.NewStreamWriter(idl.Chunk_STDOUT)
	cmd.Stderr = mux.NewStreamWriter(idl.Chunk_STDERR)

	return cmd.Run()
}
