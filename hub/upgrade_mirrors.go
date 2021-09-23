// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"bytes"
	"fmt"
	"os"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

func UpgradeMirrors(streams step.OutStreams, intermediate *greenplum.Cluster, useHbaHostnames bool) (err error) {
	err = writeAddMirrorsConfig(intermediate)
	if err != nil {
		return err
	}

	args := []string{"-a", "-i", utils.GetAddMirrorsConfig()}
	if useHbaHostnames {
		args = append(args, "--hba-hostnames")
	}

	err = intermediate.RunGreenplumCmd(streams, "gpaddmirrors", args...)
	if err != nil {
		return err
	}

	return nil
}

func writeAddMirrorsConfig(intermediate *greenplum.Cluster) error {
	var config bytes.Buffer
	for _, m := range intermediate.Mirrors {
		if m.IsStandby() {
			continue
		}

		_, err := fmt.Fprintf(&config, "%d|%s|%d|%s\n", m.ContentID, m.Hostname, m.Port, m.DataDir)
		if err != nil {
			return err
		}
	}

	err := os.WriteFile(utils.GetAddMirrorsConfig(), config.Bytes(), 0644)
	if err != nil {
		return err
	}

	return nil
}
