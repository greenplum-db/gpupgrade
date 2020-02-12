package hub

import (
	"fmt"
	"github.com/pkg/errors"
	"os/exec"

	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

var isPostmasterRunningCmd = exec.Command
var startStopCmd = exec.Command

func IsPostmasterRunning(stream step.OutStreams, cluster *utils.Cluster) error {
	cmd := isPostmasterRunningCmd("bash", "-c",
		fmt.Sprintf("pgrep -F %s/postmaster.pid",
			cluster.MasterDataDir(),
		))

	cmd.Stdout = stream.Stdout()
	cmd.Stderr = stream.Stderr()

	return cmd.Run()
}

func StartCluster(streams step.OutStreams, cluster *utils.Cluster, isSource bool) error {
	return stopStartGpdb(streams, cluster, isSource, false, false)
}

func StopCluster(streams step.OutStreams, cluster *utils.Cluster, isSource bool) error {
	return stopStartGpdb(streams, cluster, isSource, true, false)
}

func StartMasterOnly(streams step.OutStreams, cluster *utils.Cluster, isSource bool) error {
	return stopStartGpdb(streams, cluster, isSource, false, true)
}

func StopMasterOnly(streams step.OutStreams, cluster *utils.Cluster, isSource bool) error {
	return stopStartGpdb(streams, cluster, isSource, true, true)
}

func stopStartGpdb(stream step.OutStreams, cluster *utils.Cluster, isSource, isStop, isMaster bool) error {
	// TODO: why can't we call IsPostmasterRunning for the !stop case?  If we do, we get this on the pipeline:
	// Usage: pgrep [-flvx] [-d DELIM] [-n|-o] [-P PPIDLIST] [-g PGRPLIST] [-s SIDLIST]
	// [-u EUIDLIST] [-U UIDLIST] [-G GIDLIST] [-t TERMLIST] [PATTERN]
	//  pgrep: pidfile not valid
	// TODO: should we actually return an error if we try to gpstop an already stopped cluster?
	gpdbCmd := "gpstart"
	if isStop {
		gpdbCmd = "gpstop"
		err := IsPostmasterRunning(stream, cluster)
		if err != nil {
			return err
		}
	}

	masterOnlyFlag := ""
	if isMaster {
		masterOnlyFlag = "-m"
	}

	cmd := startStopCmd("bash", "-c",
		fmt.Sprintf("source %[1]s/../greenplum_path.sh && %[1]s/%[2]s %[3]s -a -d %[4]s",
			cluster.BinDir,
			gpdbCmd,
			masterOnlyFlag,
			cluster.MasterDataDir()))

	cmd.Stdout = stream.Stdout()
	cmd.Stderr = stream.Stderr()

	err := cmd.Run()
	if err != nil {
		destination := "target"
		if isSource {
			destination = "source"
		}

		operation := "start"
		if isStop {
			operation = "stop"
		}

		mode := "cluster"
		if isMaster {
			mode = "master"
		}

		message := fmt.Sprintf("failed to %s %s %s", operation, destination, mode)
		return errors.Wrap(err, message)
	}
	return nil
}
