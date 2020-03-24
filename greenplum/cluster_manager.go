package greenplum

import (
	"fmt"
	"os/exec"
)

type gpUtilities struct {
	cluster      *Cluster
	streams      OutStreams
	pgrepCommand *pgrepCommand
}

var startStopCmd = exec.Command

func newGpUtilities(cluster *Cluster, streams OutStreams) *gpUtilities {
	return &gpUtilities{
		cluster: cluster,
		streams: streams,
		pgrepCommand: &pgrepCommand{
			streams: streams,
		},
	}
}

func (m *gpUtilities) start() error {
	return m.runStartStopCmd(
		fmt.Sprintf("gpstart -a -d %[1]s", m.cluster.MasterDataDir()),
	)
}

func (m *gpUtilities) stopMasterOnly() error {
	// TODO: why can't we call isPostmasterRunning for the !stop case?  If we do, we get this on the pipeline:
	// Usage: pgrep [-flvx] [-d DELIM] [-n|-o] [-P PPIDLIST] [-g PGRPLIST] [-s SIDLIST]
	// [-u EUIDLIST] [-U UIDLIST] [-G GIDLIST] [-t TERMLIST] [PATTERN]
	//  pgrep: pidfile not valid
	// TODO: should we actually return an error if we try to gpstop an already stopped cluster?
	err := m.pgrepCommand.isRunning(m.cluster.MasterPidFile())

	if err != nil {
		return err
	}

	return m.runStartStopCmd(
		fmt.Sprintf("gpstop -m -a -d %[1]s", m.cluster.MasterDataDir()))
}

func (m *gpUtilities) startMasterOnly() error {
	return m.runStartStopCmd(
		fmt.Sprintf("gpstart -m -a -d %[1]s", m.cluster.MasterDataDir()))
}

func (m *gpUtilities) stop() error {
	// TODO: why can't we call isPostmasterRunning for the !stop case?  If we do, we get this on the pipeline:
	// Usage: pgrep [-flvx] [-d DELIM] [-n|-o] [-P PPIDLIST] [-g PGRPLIST] [-s SIDLIST]
	// [-u EUIDLIST] [-U UIDLIST] [-G GIDLIST] [-t TERMLIST] [PATTERN]
	//  pgrep: pidfile not valid
	// TODO: should we actually return an error if we try to gpstop an already stopped cluster?
	err := m.pgrepCommand.isRunning(m.cluster.MasterPidFile())

	if err != nil {
		return err
	}

	return m.runStartStopCmd(
		fmt.Sprintf("gpstop -a -d %[1]s", m.cluster.MasterDataDir()))
}

/*
 * Helper functions
 */
func (m *gpUtilities) runStartStopCmd(command string) error {
	commandWithEnv := fmt.Sprintf("source %[1]s/../greenplum_path.sh && %[1]s/%[2]s",
		m.cluster.BinDir,
		command)

	cmd := startStopCmd("bash", "-c", commandWithEnv)
	cmd.Stdout = m.streams.Stdout()
	cmd.Stderr = m.streams.Stderr()
	return cmd.Run()
}
