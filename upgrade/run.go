package upgrade

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

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

	// XXX ...but we make a single exception for now, for LD_LIBRARY_PATH, to
	// work around pervasive problems with RPATH settings in our Postgres
	// extension modules.
	if path, ok := os.LookupEnv("LD_LIBRARY_PATH"); ok {
		cmd.Env = append(cmd.Env, fmt.Sprintf("LD_LIBRARY_PATH=%s", path))
	}

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

// WithExecCommand tells Run to use the provided function to obtain an exec.Cmd
// for execution. This is provided so that callers that use the exectest package
// may stub out execution of pg_upgrade during testing.
func WithExecCommand(execCommand func(string, ...string) *exec.Cmd) Option {
	return func(o *optionList) {
		o.ExecCommand = execCommand
		o.ExecCommandSet = true
	}
}

// optionList holds the combined result of all possible Options. Zero values
// represent the default settings.
type optionList struct {
	Dir            string
	CheckOnly      bool
	ExecCommand    func(string, ...string) *exec.Cmd
	ExecCommandSet bool // was ExecCommand explicitly set?
	SegmentMode    bool
	Stdout, Stderr io.Writer
}

// newOptionList returns an optionList with all of the provided Options applied.
func newOptionList(opts []Option) *optionList {
	options := new(optionList)
	for _, opt := range opts {
		opt(options)
	}
	return options
}
