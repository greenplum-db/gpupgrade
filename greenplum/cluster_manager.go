package greenplum

import (
	"os/exec"
)

type gpStart struct {
	cluster *Cluster
	runner  Runner
}

type gpStop struct {
	cluster      *Cluster
	runner       Runner
	pgrepCommand *pgrepCommand
}

var startStopCmd = exec.Command

func newGpStart(cluster *Cluster, runner Runner) *gpStart {
	return &gpStart{
		cluster: cluster,
		runner:  runner,
	}
}

func newGpStop(cluster *Cluster, runner Runner, pgrepCommand *pgrepCommand) *gpStop {
	return &gpStop{
		cluster:      cluster,
		runner:       runner,
		pgrepCommand: pgrepCommand,
	}
}

func (m *gpStart) Start() error {
	return m.runner.Run("gpstart", "-a", "-d", m.cluster.MasterDataDir())
}

func (m *gpStop) StopMasterOnly() error {
	// TODO: why can't we call isPostmasterRunning for the !stop case?  If we do, we get this on the pipeline:
	// Usage: pgrep [-flvx] [-d DELIM] [-n|-o] [-P PPIDLIST] [-g PGRPLIST] [-s SIDLIST]
	// [-u EUIDLIST] [-U UIDLIST] [-G GIDLIST] [-t TERMLIST] [PATTERN]
	//  pgrep: pidfile not valid
	// TODO: should we actually return an error if we try to gpstop an already stopped cluster?
	err := m.pgrepCommand.isRunning(m.cluster.MasterPidFile())

	if err != nil {
		return err
	}

	return m.runner.Run("gpstop", "-m", "-a", "-d", m.cluster.MasterDataDir())
}

func (m *gpStart) StartMasterOnly() error {
	return m.runner.Run("gpstart", "-m", "-a", "-d", m.cluster.MasterDataDir())
}

func (m *gpStop) Stop() error {
	// TODO: why can't we call isPostmasterRunning for the !stop case?  If we do, we get this on the pipeline:
	// Usage: pgrep [-flvx] [-d DELIM] [-n|-o] [-P PPIDLIST] [-g PGRPLIST] [-s SIDLIST]
	// [-u EUIDLIST] [-U UIDLIST] [-G GIDLIST] [-t TERMLIST] [PATTERN]
	//  pgrep: pidfile not valid
	// TODO: should we actually return an error if we try to gpstop an already stopped cluster?
	err := m.pgrepCommand.isRunning(m.cluster.MasterPidFile())

	if err != nil {
		return err
	}

	return m.runner.Run("gpstop", "-a", "-d", m.cluster.MasterDataDir())
}
