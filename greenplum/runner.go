// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/kballard/go-shellquote"

	"github.com/greenplum-db/gpupgrade/step"
)

type Runner interface {
	Run(utilityName string, arguments ...string) error
}

func NewRunner(c *Cluster, streams step.OutStreams) Runner {
	return &runner{
		masterPort:          c.MasterPort(),
		masterDataDirectory: c.MasterDataDir(),
		gphome:              c.GPHome,
		streams:             streams,
	}
}

func (e *runner) Run(utilityName string, arguments ...string) error {
	path := filepath.Join(e.gphome, "bin", utilityName)

	arguments = append([]string{path}, arguments...)
	script := shellquote.Join(arguments...)

	withGreenplumPath := fmt.Sprintf("source %s/greenplum_path.sh && %s", e.gphome, script)
	gplog.Debug(withGreenplumPath)

	command := exec.Command("bash", "-c", withGreenplumPath)
	command.Env = append(command.Env, fmt.Sprintf("%v=%v", "MASTER_DATA_DIRECTORY", e.masterDataDirectory))
	command.Env = append(command.Env, fmt.Sprintf("%v=%v", "PGPORT", e.masterPort))

	command.Stdout = e.streams.Stdout()
	command.Stderr = e.streams.Stderr()

	return command.Run()
}

type runner struct {
	gphome              string
	masterDataDirectory string
	masterPort          int
	streams             step.OutStreams
}
