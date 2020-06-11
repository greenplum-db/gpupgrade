// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package rsync

import (
	"os/exec"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
)

var rsyncCommand = exec.Command

// Rsync executes "rsync" on the system with the given options.
// The caller needs to consider adding "/" using os.PathSeparator to the
// source directory to control where in the destination directory the
// files go.
//
// Any errors returned are of type RsyncError, which wraps the underlying
// error from "rsync" if called.
func Rsync(options ...Option) error {
	opts := newOptionList(options...)

	dstPath := opts.destination
	if opts.hasRemoteHost {
		dstPath = opts.remoteHost + ":" + opts.destination
	}

	var args []string
	args = append(args, opts.options...)
	args = append(args, opts.sources...)
	args = append(args, dstPath)
	args = append(args, opts.excludedFiles...)

	cmd := rsyncCommand("rsync", args...)

	// when no streams are specified, capture stderr for the error message
	stream := step.BufferedStreams{}
	cmd.Stderr = stream.Stderr()
	if opts.useStream {
		cmd.Stdout = opts.stream.Stdout()
		cmd.Stderr = opts.stream.Stderr()
	}

	gplog.Info("running Rsync as %s", cmd.String())

	err := cmd.Run()
	if err != nil {
		errorText := err.Error()

		// bubble up the rsync error with the underlying cause
		if !opts.useStream && stream.StderrBuf.String() != "" {
			errorText = stream.StderrBuf.String()
		}

		return RsyncError{errorText: errorText, err: err}
	}

	return nil
}

// XXX: for internal testing only
func SetRsyncCommand(command exectest.Command) {
	rsyncCommand = command
}

func ResetRsyncCommand() {
	rsyncCommand = exec.Command
}

type RsyncError struct {
	errorText string
	err       error // underlying error of rsync call
}

func (e RsyncError) Error() string {
	return e.errorText
}

func (e RsyncError) Unwrap() error {
	return e.err
}

type Option func(*optionList)

func WithSources(srcs ...string) Option {
	return func(options *optionList) {
		options.sources = append(options.sources, srcs...)
	}
}

func WithRemoteHost(host string) Option {
	return func(options *optionList) {
		options.hasRemoteHost = true
		options.remoteHost = host
	}
}

func WithDestination(dst string) Option {
	return func(options *optionList) {
		options.destination = dst
	}
}

func WithOptions(opts ...string) Option {
	return func(options *optionList) {
		options.options = append(options.options, opts...)
	}
}

func WithExcludedFiles(files ...string) Option {
	return func(options *optionList) {
		for _, excludedFile := range files {
			options.excludedFiles = append(options.excludedFiles, "--exclude", excludedFile)
		}
	}
}

func WithStream(stream step.OutStreams) Option {
	return func(options *optionList) {
		options.stream = stream
		options.useStream = true
	}
}

type optionList struct {
	sources       []string
	hasRemoteHost bool
	remoteHost    string
	destination   string
	options       []string
	excludedFiles []string
	useStream     bool
	stream        step.OutStreams
}

func newOptionList(opts ...Option) *optionList {
	o := new(optionList)
	for _, option := range opts {
		option(o)
	}
	return o
}
