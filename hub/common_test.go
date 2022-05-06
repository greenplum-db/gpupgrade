// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils/disk"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

// Set it to nil so we don't accidentally execute a command for real during tests
func init() {
	ResetExecCommand()
	rsync.ResetRsyncCommand()
	ResetCheckDiskUsage()

	exectest.RegisterMains(
		Success,
		Failure,
		StreamingMain,
		EnvironmentMain,
	)
}

func Success() {}

func Failure() {
	os.Stderr.WriteString(os.ErrPermission.Error())
	os.Exit(1)
}

const StreamingMainStdout = "expected\nstdout\n"
const StreamingMainStderr = "process\nstderr\n"

// Streams the above stdout/err constants to the corresponding standard file
// descriptors, alternately interleaving five-byte chunks.
func StreamingMain() {
	stdout := bytes.NewBufferString(StreamingMainStdout)
	stderr := bytes.NewBufferString(StreamingMainStderr)

	for stdout.Len() > 0 || stderr.Len() > 0 {
		os.Stdout.Write(stdout.Next(5))
		os.Stderr.Write(stderr.Next(5))
	}
}

// Prints the environment, one variable per line, in NAME=VALUE format.
func EnvironmentMain() {
	for _, e := range os.Environ() {
		fmt.Println(e)
	}
}

func SetExecCommand(cmdFunc exectest.Command) {
	ExecCommand = cmdFunc
}

func ResetExecCommand() {
	ExecCommand = nil
}

func SetCheckDiskUsage(usageFunc disk.CheckUsageType) {
	checkDiskUsage = usageFunc
}

func ResetCheckDiskUsage() {
	checkDiskUsage = disk.CheckUsage
}

// MustCreateCluster creates a utils.Cluster and calls t.Fatalf() if there is
// any error.
func MustCreateCluster(t *testing.T, segments greenplum.SegConfigs) *greenplum.Cluster {
	t.Helper()

	cluster, err := greenplum.NewCluster(segments)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	return &cluster
}
