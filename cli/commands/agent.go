// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"github.com/spf13/cobra"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/daemon"
	"github.com/greenplum-db/gpupgrade/utils/logger"
)

func Agent() *cobra.Command {
	var agentPort int
	var stateDir string
	var shouldDaemonize bool

	var cmd = &cobra.Command{
		Use:    "agent",
		Short:  "start the agent",
		Long:   "start the agent",
		Hidden: true,
		Args:   cobra.MaximumNArgs(0), // no positional args allowed
		RunE: func(cmd *cobra.Command, args []string) error {
			logger.Initialize("agent")
			defer logger.WritePanics()

			agentServer := agent.New()

			// blocking call
			return agentServer.Start(agentPort, stateDir, shouldDaemonize)
		},
	}

	cmd.Flags().IntVar(&agentPort, "port", upgrade.DefaultAgentPort, "the port to listen for commands on")
	cmd.Flags().StringVar(&stateDir, "state-directory", utils.GetStateDir(), "Agent state directory")

	daemon.MakeDaemonizable(cmd, &shouldDaemonize)

	return cmd
}
