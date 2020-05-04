// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/spf13/cobra"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/daemon"
	"github.com/greenplum-db/gpupgrade/utils/log"
)

func Agent() *cobra.Command {
	var statedir string
	var shouldDaemonize bool

	var cmd = &cobra.Command{
		Use:    "agent",
		Short:  "Start the Command Listener (blocks)",
		Long:   `Start the Command Listener (blocks)`,
		Hidden: true,
		Args:   cobra.MaximumNArgs(0), //no positional args allowed
		RunE: func(cmd *cobra.Command, args []string) error {
			gplog.InitializeLogging("gpupgrade agent", "")
			defer log.WritePanics()

			conf := agent.Config{
				Port:     6416,
				StateDir: statedir,
			}

			agentServer := agent.NewServer(conf)
			if shouldDaemonize {
				agentServer.MakeDaemon()
			}

			// blocking call
			agentServer.Start()

			return nil
		},
	}

	cmd.Flags().StringVar(&statedir, "state-directory", utils.GetStateDir(), "Agent state directory")

	daemon.MakeDaemonizable(cmd, &shouldDaemonize)

	return cmd
}
