package hub

import (
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

// Allow exec.Command to be mocked out by exectest.NewCommand.
var execCommand = exec.Command
var execCommandRsync = exec.Command

const masterBackup = "master.bak"

// XXX this makes more sense as a Hub method, but it's so difficult to stub a
// Hub that the parameters have been split out for testing. Revisit if/when the
// Hub monolith is broken up.
func UpgradeMaster(source, target *utils.Cluster, stateDir string, stream step.OutStreams, checkOnly bool, useLinkMode bool) error {
	wd := utils.MasterPGUpgradeDirectory(stateDir)
	err := utils.System.MkdirAll(wd, 0700)
	if err != nil {
		return err
	}

	sourceDir := filepath.Join(stateDir, masterBackup)
	err = RsyncMasterDataDir(stream, sourceDir, target.MasterDataDir())
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

func RsyncMasterDataDir(stream step.OutStreams, sourceDir, targetDir string) error {
	sourceDirRsync := filepath.Clean(sourceDir) + string(os.PathSeparator)
	cmd := execCommandRsync("rsync", "--archive", "--exclude=pg_log/*", sourceDirRsync, targetDir)

	cmd.Stdout = stream.Stdout()
	cmd.Stderr = stream.Stderr()

	err := cmd.Run()
	if err != nil {
		return xerrors.Errorf("rsync %q to %q: %w", sourceDirRsync, targetDir, err)
	}
	return nil
}
