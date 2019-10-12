package services

import (
	"fmt"
	"github.com/greenplum-db/gpupgrade/idl"
	"io"
	"os/exec"

	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
)

var stopClusterCmd = exec.Command
var isPostmasterRunningCmd = exec.Command

func (h *Hub) ShutdownClusters(stream messageSender, log io.Writer) error {
	var shutdownErr error

	err := StopCluster(stream, log, h.source)
	if err != nil {
		shutdownErr = multierror.Append(shutdownErr, errors.Wrap(err, "failed to stop source cluster"))
	}

	err = StopCluster(stream, log, h.target)
	if err != nil {
		shutdownErr = multierror.Append(shutdownErr, errors.Wrap(err, "failed to stop target cluster"))
	}

	return shutdownErr
}

func StopCluster(stream messageSender, log io.Writer, cluster *utils.Cluster) error {
	err := IsPostmasterRunning(stream, log, cluster)
	if err != nil {
		return err
	}

	cmd := stopClusterCmd("bash", "-c",
		fmt.Sprintf("source %[1]s/../greenplum_path.sh && %[1]s/gpstop -a -d %[2]s",
			cluster.BinDir,
			cluster.MasterDataDir(),
		))

	mux := newMultiplexedStream(stream, log)
	cmd.Stdout = mux.NewStreamWriter(idl.Chunk_STDOUT)
	cmd.Stderr = mux.NewStreamWriter(idl.Chunk_STDERR)

	return cmd.Run()
}

func IsPostmasterRunning(stream messageSender,  log io.Writer, cluster *utils.Cluster) error {
	cmd := isPostmasterRunningCmd("bash", "-c",
		fmt.Sprintf("pgrep -F %s/postmaster.pid",
			cluster.MasterDataDir(),
		))

	mux := newMultiplexedStream(stream, log)
	cmd.Stdout = mux.NewStreamWriter(idl.Chunk_STDOUT)
	cmd.Stderr = mux.NewStreamWriter(idl.Chunk_STDERR)

	return cmd.Run()
}