// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/kballard/go-shellquote"
)

type Runner interface {
	Run(utilityName string, arguments ...string) error
}

type OutStreams interface {
	Stdout() io.Writer
	Stderr() io.Writer
}

func NewRunner(c *Cluster, streams OutStreams) Runner {
	return &runner{
		masterPort:          c.MasterPort(),
		masterDataDirectory: c.MasterDataDir(),
		binDir:              c.BinDir,
		streams:             streams,
	}
}

func (e *runner) Run(utilityName string, arguments ...string) error {
	path := filepath.Join(e.binDir, utilityName)

	arguments = append([]string{path}, arguments...)
	script := shellquote.Join(arguments...)

	withGreenplumPath := fmt.Sprintf("source %s/../greenplum_path.sh && %s", e.binDir, script)
	gplog.Debug(withGreenplumPath)

	command := exec.Command("bash", "-c", withGreenplumPath)
	command.Env = append(command.Env, fmt.Sprintf("%v=%v", "MASTER_DATA_DIRECTORY", e.masterDataDirectory))
	command.Env = append(command.Env, fmt.Sprintf("%v=%v", "PGPORT", e.masterPort))

	command.Stdout = e.streams.Stdout()
	command.Stderr = e.streams.Stderr()

	return command.Run()
}

type runner struct {
	binDir              string
	masterDataDirectory string
	masterPort          int

	streams OutStreams
}
