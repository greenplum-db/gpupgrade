package hub

import (
	"os/exec"

	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

// Allow exec.Command to be mocked out by exectest.NewCommand.
var execCommand = exec.Command

// XXX this makes more sense as a Hub method, but it's so difficult to stub a
// Hub that the parameters have been split out for testing. Revisit if/when the
// Hub monolith is broken up.
func UpgradeMaster(source, target *utils.Cluster, stateDir string, stream OutStreams, checkOnly bool, useLinkMode bool) error {
	wd := utils.MasterPGUpgradeDirectory(stateDir)
	err := utils.System.MkdirAll(wd, 0700)
	if err != nil {
		return err
	}

	pair := upgrade.SegmentPair{
		Source: masterSegmentFromCluster(source),
		Target: masterSegmentFromCluster(target),
	}

	options := []upgrade.Option{
		upgrade.WithExecCommand(execCommand),
		upgrade.WithWorkDir(wd),
		upgrade.WithOutputStreams(stream.Stdout(), stream.Stderr()),
	}
	if checkOnly {
		options = append(options, upgrade.WithCheckOnly())
	}

	if useLinkMode {
		options = append(options, upgrade.WithLinkMode())
	}

	return upgrade.Run(pair, options...)
}

func masterSegmentFromCluster(cluster *utils.Cluster) *upgrade.Segment {
	return &upgrade.Segment{
		BinDir:  cluster.BinDir,
		DataDir: cluster.MasterDataDir(),
		DBID:    cluster.GetDbidForContent(-1),
		Port:    cluster.MasterPort(),
	}
}
