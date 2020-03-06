package hub

import (
	"fmt"
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

func StartCluster(stream step.OutStreams, cluster *utils.Cluster) error {
	return runStartStopCmd(stream,
		fmt.Sprintf("source %[1]s/../greenplum_path.sh && %[1]s/%[2]s %[3]s -a -d %[4]s",
			cluster.BinDir,
			"gpstart",
			"",
			cluster.MasterDataDir()))
}

func StopCluster(stream step.OutStreams, cluster *utils.Cluster) error {
	// TODO: why can't we call IsPostmasterRunning for the !stop case?  If we do, we get this on the pipeline:
	// Usage: pgrep [-flvx] [-d DELIM] [-n|-o] [-P PPIDLIST] [-g PGRPLIST] [-s SIDLIST]
	// [-u EUIDLIST] [-U UIDLIST] [-G GIDLIST] [-t TERMLIST] [PATTERN]
	//  pgrep: pidfile not valid
	// TODO: should we actually return an error if we try to gpstop an already stopped cluster?
	err := IsPostmasterRunning(stream, cluster)
	if err != nil {
		return err
	}

	return runStartStopCmd(stream,
		fmt.Sprintf("source %[1]s/../greenplum_path.sh && %[1]s/%[2]s -a -d %[3]s",
			cluster.BinDir,
			"gpstop",
			cluster.MasterDataDir()))
}

func StartMasterOnly(stream step.OutStreams, cluster *utils.Cluster) error {
	return runStartStopCmd(stream,
		fmt.Sprintf("source %[1]s/../greenplum_path.sh && %[1]s/%[2]s %[3]s -a -d %[4]s",
			cluster.BinDir,
			"gpstart",
			"-m",
			cluster.MasterDataDir()))
}

func StopMasterOnly(stream step.OutStreams, cluster *utils.Cluster) error {
	// TODO: why can't we call IsPostmasterRunning for the !stop case?  If we do, we get this on the pipeline:
	// Usage: pgrep [-flvx] [-d DELIM] [-n|-o] [-P PPIDLIST] [-g PGRPLIST] [-s SIDLIST]
	// [-u EUIDLIST] [-U UIDLIST] [-G GIDLIST] [-t TERMLIST] [PATTERN]
	//  pgrep: pidfile not valid
	// TODO: should we actually return an error if we try to gpstop an already stopped cluster?
	err := IsPostmasterRunning(stream, cluster)
	if err != nil {
		return err
	}

	return runStartStopCmd(stream,
		fmt.Sprintf("source %[1]s/../greenplum_path.sh && %[1]s/%[2]s %[3]s -a -d %[4]s",
			cluster.BinDir,
			"gpstop",
			"-m",
			cluster.MasterDataDir()))
}

func runStartStopCmd(stream step.OutStreams, command string) error {
	cmd := startStopCmd("bash", "-c", command)
	cmd.Stdout = stream.Stdout()
	cmd.Stderr = stream.Stderr()
	return cmd.Run()
}
