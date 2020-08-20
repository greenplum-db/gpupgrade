// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"io"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

const DefaultHubPort = 7527
const DefaultAgentPort = 6416

// execCommand allows tests to stub out the Commands that are actually run. See
// also the WithExecCommand option.
var execCommand = exec.Command

// Segment holds the information needed to upgrade a single GPDB segment.
type Segment struct {
	BinDir  string
	DataDir string
	DBID    int
	Port    int
}

// SegmentPair holds an upgradable pair of Segments.
type SegmentPair struct {
	Source, Target *Segment
}

// Run executes pg_upgrade for the given pair of Segments. By default, a
// standard master upgrade is performed; this can be changed by passing various
// Options.
func Run(p SegmentPair, options ...Option) error {
	opts := newOptionList(options)

	mode := "dispatcher"
	if opts.SegmentMode {
		mode = "segment"
	}

	path := filepath.Join(p.Target.BinDir, "pg_upgrade")
	args := []string{
		"--retain", // always keep log files around
		"--old-bindir", p.Source.BinDir,
		"--new-bindir", p.Target.BinDir,
		"--old-gp-dbid", strconv.Itoa(p.Source.DBID),
		"--new-gp-dbid", strconv.Itoa(p.Target.DBID),
		"--old-datadir", p.Source.DataDir,
		"--new-datadir", p.Target.DataDir,
		"--old-port", strconv.Itoa(p.Source.Port),
		"--new-port", strconv.Itoa(p.Target.Port),
		"--mode", mode,
	}

	if opts.CheckOnly {
		args = append(args, "--check")
	}

	if opts.UseLinkMode {
		args = append(args, "--link")
	}

	if opts.TablespaceFilePath != "" {
		args = append(args, "--old-tablespaces-file", opts.TablespaceFilePath)
	}

	if opts.OldOptions != "" {
		args = append(args, "--old-options", opts.OldOptions)
	}

	// If the caller specified an explicit Command implementation to use, get
	// our exec.Cmd using that. Otherwise use our internal execCommand.
	cmdFunc := execCommand
	if opts.ExecCommandSet {
		cmdFunc = opts.ExecCommand
	}
	cmd := cmdFunc(path, args...)

	cmd.Dir = opts.Dir
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr

	// Explicitly clear the child environment. pg_upgrade shouldn't need things
	// like PATH, and PGPORT et al are explicitly forbidden to be set.
	cmd.Env = []string{}

	// XXX Use this environment variable to tell pg_upgrade to include timing information
	// in its returned output.  We would prefer to use the flag `--print-timing` but that
	// would require coordination with a new release of GPDB-6 as well as a bump in the
	// minimum version of GPDB-6 required for gpupgrade.  Once we have bumped the minimum
	// version of GPDB-6 past a release that includes the new print-timing code, we can
	// migrate to the flag.  See https://github.com/greenplum-db/gpdb/pull/10661.
	cmd.Env = append(cmd.Env, "__GPDB_PGUPGRADE_PRINT_TIMING__=1")

	gplog.Info(cmd.String())

	return cmd.Run()
}

// Option configures the way Run executes pg_upgrade.
type Option func(*optionList)

// WithWorkDir configures the working directory for pg_upgrade. Log files will
// be retained in this directory. If this option is unset or empty, the calling
// process's working directory will be used.
func WithWorkDir(wd string) Option {
	return func(o *optionList) {
		o.Dir = wd
	}
}

// WithOutputStreams configures the stdout/stderr io.Writer streams for
// pg_upgrade. If this option is unset (or if either io.Writer is set to nil),
// the output will be discarded.
func WithOutputStreams(out, err io.Writer) Option {
	return func(o *optionList) {
		o.Stdout = out
		o.Stderr = err
	}
}

// WithSegmentMode configures pg_upgrade for segment-mode execution
// (--mode=segment). By default, an upgrade is performed for a master
// (--mode=dispatcher).
func WithSegmentMode() Option {
	return func(o *optionList) {
		o.SegmentMode = true
	}
}

// WithCheckOnly configures pg_upgrade to only run preflight checks (--check).
func WithCheckOnly() Option {
	return func(o *optionList) {
		o.CheckOnly = true
	}
}

// WithLinkMode allows pg_upgrade to run upgrade with --link mode
func WithLinkMode() Option {
	return func(o *optionList) {
		o.UseLinkMode = true
	}
}

// WithExecCommand tells Run to use the provided function to obtain an exec.Cmd
// for execution. This is provided so that callers that use the exectest package
// may stub out execution of pg_upgrade during testing.
func WithExecCommand(execCommand func(string, ...string) *exec.Cmd) Option {
	return func(o *optionList) {
		o.ExecCommand = execCommand
		o.ExecCommandSet = true
	}
}

// WithTablespaceFile configures the tablespace mapping file path passed to pg_upgrade
// to perform the upgrade of the segment tablespaces.
func WithTablespaceFile(filePath string) Option {
	return func(o *optionList) {
		o.TablespaceFilePath = filePath
	}
}

// WithOldOptions allows the "--old-options" flag to pg_upgrade to be set by the caller,
// to set extra options on the pg_ctl used for the source cluster.
func WithOldOptions(opts string) Option {
	return func(o *optionList) {
		o.OldOptions = opts
	}
}

// optionList holds the combined result of all possible Options. Zero values
// represent the default settings.
type optionList struct {
	Dir                string
	CheckOnly          bool
	UseLinkMode        bool
	ExecCommand        func(string, ...string) *exec.Cmd
	ExecCommandSet     bool // was ExecCommand explicitly set?
	SegmentMode        bool
	Stdout, Stderr     io.Writer
	TablespaceFilePath string
	OldOptions         string
}

// newOptionList returns an optionList with all of the provided Options applied.
func newOptionList(opts []Option) *optionList {
	options := new(optionList)
	for _, opt := range opts {
		opt(options)
	}
	return options
}
