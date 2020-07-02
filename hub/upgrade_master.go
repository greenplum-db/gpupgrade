// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

// Allow exec.Command to be mocked out by exectest.NewCommand.
var execCommand = exec.Command

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

	// Buffer stdout to add context to errors.
	stdout := new(bytes.Buffer)
	tee := io.MultiWriter(args.Stream.Stdout(), stdout)

	options := []upgrade.Option{
		upgrade.WithExecCommand(execCommand),
		upgrade.WithWorkDir(wd),
		upgrade.WithOutputStreams(tee, args.Stream.Stderr()),
	}
	if args.CheckOnly {
		options = append(options, upgrade.WithCheckOnly())
	}

	if args.UseLinkMode {
		options = append(options, upgrade.WithLinkMode())
	}

	err = upgrade.Run(pair, options...)
	if err != nil {
		// Error details from stdout are added to any errors containing "fatal"
		// such as pg_ugprade check errors.
		var text []string
		var addText bool

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasSuffix(line, "fatal") || addText {
				addText = true
				text = append(text, line)
			}
		}

		return UpgradeMasterError{ErrorText: strings.Join(text, "\n"), err: err}
	}

	return nil
}

type UpgradeMasterError struct {
	ErrorText string
	err       error
}

func (u UpgradeMasterError) Error() string {
	if u.ErrorText == "" {
		return fmt.Sprintf("upgrading master: %v", u.err)
	}

	return fmt.Sprintf("upgrading master: %s: %v", u.ErrorText, u.err)
}

func (u UpgradeMasterError) Unwrap() error {
	return u.err
}

func masterSegmentFromCluster(cluster *greenplum.Cluster) *upgrade.Segment {
	return &upgrade.Segment{
		BinDir:  filepath.Join(cluster.GPHome, "bin"),
		DataDir: cluster.MasterDataDir(),
		DBID:    cluster.GetDbidForContent(-1),
		Port:    cluster.MasterPort(),
	}
}

func RsyncMasterDataDir(stream step.OutStreams, sourceDir, targetDir string) error {
	sourceDirRsync := filepath.Clean(sourceDir) + string(os.PathSeparator)

	options := []rsync.Option{
		rsync.WithSources(sourceDirRsync),
		rsync.WithDestination(targetDir),
		rsync.WithOptions("--archive", "--delete"),
		rsync.WithExcludedFiles("pg_log/*"),
		rsync.WithStream(stream),
	}

	err := rsync.Rsync(options...)
	if err != nil {
		return xerrors.Errorf("rsync %q to %q: %w", sourceDirRsync, targetDir, err)
	}

	return nil
}
