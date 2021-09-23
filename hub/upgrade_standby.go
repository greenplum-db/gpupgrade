// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"strconv"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/step"
)

// UpgradeStandby removes any possible existing standby from the cluster
// before adding a new one for idempotency. In the happy-path, we expect this to
// fail as there should not be an existing  standby for the cluster.
func UpgradeStandby(streams step.OutStreams, intermediate *greenplum.Cluster, useHbaHostnames bool) error {
	gplog.Info("removing any existing standby master on target cluster")
	err := intermediate.RunGreenplumCmd(streams, "gpinitstandby", "-r", "-a")
	if err != nil {
		gplog.Debug("error message from removing existing standby master (expected in the happy path): %v", err)
	}

	args := []string{
		"-P", strconv.Itoa(intermediate.Standby().Port),
		"-s", intermediate.Standby().Hostname,
		"-S", intermediate.Standby().DataDir,
		"-a",
	}

	if useHbaHostnames {
		args = append(args, "--hba-hostnames")
	}

	return intermediate.RunGreenplumCmd(streams, "gpinitstandby", args...)
}
