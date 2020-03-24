package greenplum

import (
	"os/exec"
)

type gpStart struct {
	cluster *Cluster
	runner  Runner
}

var startStopCmd = exec.Command

func newGpStart(cluster *Cluster, runner Runner) *gpStart {
	return &gpStart{
		cluster: cluster,
		runner:  runner,
	}
}

func (m *gpStart) Start() error {
	return m.runner.Run("gpstart", "-a", "-d", m.cluster.MasterDataDir())
}

func (m *gpStart) StartMasterOnly() error {
	return m.runner.Run("gpstart", "-m", "-a", "-d", m.cluster.MasterDataDir())
}
