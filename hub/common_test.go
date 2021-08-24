// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
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
}

func SetExecCommand(cmdFunc exectest.Command) {
	execCommand = cmdFunc
}

func ResetExecCommand() {
	execCommand = nil
}

func SetCheckDiskUsage(usageFunc disk.CheckUsageType) {
	checkDiskUsage = usageFunc
}

func ResetCheckDiskUsage() {
	checkDiskUsage = disk.CheckUsage
}

// MustCreateCluster creates a utils.Cluster and calls t.Fatalf() if there is
// any error.
func MustCreateCluster(t *testing.T, segs []greenplum.SegConfig) *greenplum.Cluster {
	t.Helper()

	cluster, err := greenplum.NewCluster(segs)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	return &cluster
}
