// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/greenplum-db/gpupgrade/config"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/daemon"
	"github.com/greenplum-db/gpupgrade/utils/logger"
)

func Hub() *cobra.Command {
	var hubPort int
	var shouldDaemonize bool

	var cmd = &cobra.Command{
		Use:    "hub",
		Short:  "start the hub",
		Long:   "start the hub",
		Hidden: true,
		Args:   cobra.MaximumNArgs(0), //no positional args allowed
		RunE: func(cmd *cobra.Command, args []string) error {
			logger.Initialize("hub")
			defer logger.WritePanics()

			exist, err := upgrade.PathExist(utils.GetStateDir())
			if err != nil {
				return err
			}

			if !exist {
				nextAction := fmt.Sprintf(`Run "gpupgrade %s" to start the hub.`, idl.Step_initialize)
				err = fmt.Errorf("gpupgrade state directory %q does not exist", utils.GetStateDir())
				return utils.NewNextActionErr(err, nextAction)
			}

			conf, err := config.Read()
			if err != nil {
				return err
			}

			// allow command line args precedence over config file values
			if cmd.Flag("port").Changed {
				conf.HubPort = hubPort
			}

			hubServer := hub.New(conf)
			return hubServer.Start(conf.HubPort, shouldDaemonize)
		},
	}

	cmd.Flags().IntVar(&hubPort, "port", upgrade.DefaultHubPort, "the port to listen for commands on")

	daemon.MakeDaemonizable(cmd, &shouldDaemonize)

	return cmd
}
