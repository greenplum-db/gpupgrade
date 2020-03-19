package hub

import (
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

// Allow exec.Command to be mocked out by exectest.NewCommand.
var execCommand = exec.Command
var execCommandRsync = exec.Command

const originalMasterBackupName = "master.bak"

type UpgradeMasterArgs struct {
	Source      *greenplum.Cluster
	Target      *greenplum.Cluster
	StateDir    string
	Stream      step.OutStreams
	CheckOnly   bool
	UseLinkMode bool
}

// XXX this makes more sense as a Server method, but it's so difficult to stub a
// Server that the parameters have been split out for testing. Revisit if/when the
// Server monolith is broken up.
func UpgradeMaster(args UpgradeMasterArgs) error {
	wd := upgrade.MasterWorkingDirectory(args.StateDir)
	err := utils.System.MkdirAll(wd, 0700)
	if err != nil {
		return err
	}

	sourceDir := filepath.Join(args.StateDir, originalMasterBackupName)
	err = RsyncMasterDataDir(args.Stream, sourceDir, args.Target.MasterDataDir())
	if err != nil {
		return err
	}

	pair := upgrade.SegmentPair{
		Source: masterSegmentFromCluster(args.Source),
		Target: masterSegmentFromCluster(args.Target),
	}

	options := []upgrade.Option{
		upgrade.WithExecCommand(execCommand),
		upgrade.WithWorkDir(wd),
		upgrade.WithOutputStreams(args.Stream.Stdout(), args.Stream.Stderr()),
	}
	if args.CheckOnly {
		options = append(options, upgrade.WithCheckOnly())
	}

	if args.UseLinkMode {
		options = append(options, upgrade.WithLinkMode())
	}

	return upgrade.Run(pair, options...)
}

func masterSegmentFromCluster(cluster *greenplum.Cluster) *upgrade.Segment {
	return &upgrade.Segment{
		BinDir:  cluster.BinDir,
		DataDir: cluster.MasterDataDir(),
		DBID:    cluster.GetDbidForContent(-1),
		Port:    cluster.MasterPort(),
	}
}

func RsyncMasterDataDir(stream step.OutStreams, sourceDir, targetDir string) error {
	sourceDirRsync := filepath.Clean(sourceDir) + string(os.PathSeparator)
	cmd := execCommandRsync("rsync", "--archive", "--delete", "--exclude=pg_log/*", sourceDirRsync, targetDir)

	cmd.Stdout = stream.Stdout()
	cmd.Stderr = stream.Stderr()

	err := cmd.Run()
	if err != nil {
		return xerrors.Errorf("rsync %q to %q: %w", sourceDirRsync, targetDir, err)
	}
	return nil
}
