// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/greenplum-db/gpupgrade/config"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/daemon"
	"github.com/greenplum-db/gpupgrade/utils/logger"
)

func Hub() *cobra.Command {
	var port int
	var shouldDaemonize bool

	var cmd = &cobra.Command{
		Use:    "hub",
		Short:  "Start the gpupgrade hub (blocks)",
		Long:   `Start the gpupgrade hub (blocks)`,
		Hidden: true,
		Args:   cobra.MaximumNArgs(0), //no positional args allowed
		RunE: func(cmd *cobra.Command, args []string) error {
			logger.Initialize("hub")
			defer logger.WritePanics()

			stateDir := utils.GetStateDir()
			finfo, err := os.Stat(stateDir)
			if os.IsNotExist(err) {
				return fmt.Errorf("gpupgrade state dir (%s) does not exist. Did you run gpupgrade initialize?", stateDir)
			} else if err != nil {
				return err
			} else if !finfo.IsDir() {
				return fmt.Errorf("gpupgrade state dir (%s) does not exist as a directory.", stateDir)
			}

			// Load the hub persistent configuration.
			//
			// they're not defined in the configuration (as happens
			// pre-initialize), we still need good defaults.
			conf := &config.Config{
				HubPort:   port,
				AgentPort: upgrade.DefaultAgentPort,
				Mode:      idl.Mode_copy,
			}

			err = conf.Load()
			if err != nil {
				return err
			}

			// allow command line args precedence over config file values
			if cmd.Flag("port").Changed {
				conf.HubPort = port
			}

			h := hub.New(conf, grpc.DialContext, stateDir)

			if shouldDaemonize {
				h.MakeDaemon()
			}

			err = h.Start()
			if err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().IntVar(&port, "port", upgrade.DefaultHubPort, "the port to listen for commands on")

	daemon.MakeDaemonizable(cmd, &shouldDaemonize)

	return cmd
}
