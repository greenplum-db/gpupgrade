package services

import (
	"fmt"
	"os/exec"

	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/utils"
)

var isPostmasterRunningCmd = exec.Command
var startStopClusterCmd = exec.Command

func (h *Hub) ShutdownCluster(stream OutStreams, isSource bool) error {
	if isSource {
		err := StopCluster(stream, h.source)
		if err != nil {
			return errors.Wrap(err, "failed to stop source cluster")
		}
	} else {
		err := StopCluster(stream, h.target)
		if err != nil {
			return errors.Wrap(err, "failed to stop target cluster")
		}
	}

	return nil
}

func StopCluster(stream OutStreams, cluster *utils.Cluster) error {
	return startStopCluster(stream, cluster, true)
}
func StartCluster(stream OutStreams, cluster *utils.Cluster) error {
	return startStopCluster(stream, cluster, false)
}

func startStopCluster(stream OutStreams, cluster *utils.Cluster, stop bool) error {
	// TODO: why can't we call IsPostmasterRunning for the !stop case?  If we do, we get this on the pipeline:
	// Usage: pgrep [-flvx] [-d DELIM] [-n|-o] [-P PPIDLIST] [-g PGRPLIST] [-s SIDLIST]
	// [-u EUIDLIST] [-U UIDLIST] [-G GIDLIST] [-t TERMLIST] [PATTERN]
	//  pgrep: pidfile not valid
	// TODO: should we actually return an error if we try to gpstop an already stopped cluster?
	cmdName := "gpstart"
	if stop {
		cmdName = "gpstop"
		err := IsPostmasterRunning(stream, cluster)
		if err != nil {
			return err
		}
	}

	cmd := startStopClusterCmd("bash", "-c",
		fmt.Sprintf("source %[1]s/../greenplum_path.sh && %[1]s/%[2]s -a -d %[3]s",
			cluster.BinDir,
			cmdName,
			cluster.MasterDataDir(),
		))

	cmd.Stdout = stream.Stdout()
	cmd.Stderr = stream.Stderr()

	return cmd.Run()
}

func IsPostmasterRunning(stream OutStreams, cluster *utils.Cluster) error {
	cmd := isPostmasterRunningCmd("bash", "-c",
		fmt.Sprintf("pgrep -F %s/postmaster.pid",
			cluster.MasterDataDir(),
		))

	cmd.Stdout = stream.Stdout()
	cmd.Stderr = stream.Stderr()

	return cmd.Run()
}
