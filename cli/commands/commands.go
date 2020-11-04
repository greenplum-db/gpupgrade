// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

/*
 *  This file generates the command-line cli that is the heart of gpupgrade.  It uses Cobra to generate
 *    the cli based on commands and sub-commands. The below in this comment block shows a notional example
 *    of how this looks to give you an idea of what the command structure looks like at the cli.  It is NOT necessarily
 *    up-to-date but is a useful as an orientation to what is going on here.
 *
 * example> gpupgrade
 * 	   2018/09/28 16:09:39 Please specify one command of: check, config, prepare, status, upgrade, or version
 *
 * example> gpupgrade check
 *      collects information and validates the target Greenplum installation can be upgraded
 *
 *      Usage:
 * 		gpupgrade check [command]
 *
 * 		Available Commands:
 * 			config       gather cluster configuration
 * 			disk-space   check that disk space usage is less than 80% on all segments
 * 			object-count count database objects and numeric objects
 * 			version      validate current version is upgradable
 *
 * 		Flags:
 * 			-h, --help   help for check
 *
 * 		Use "gpupgrade check [command] --help" for more information about a command.
 */

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

func BuildRootCommand() *cobra.Command {
	// TODO: if called without a subcommand, the cli prints a help message with timestamp.  Remove the timestamp.
	var shouldPrintVersion bool

	root := &cobra.Command{
		Use: "gpupgrade",
		RunE: func(cmd *cobra.Command, args []string) error {
			if shouldPrintVersion {
				printVersion()
				return nil
			}

			logdir, err := utils.GetLogDir()
			if err != nil {
				return xerrors.Errorf("getting log directory: %w", err)
			}

			_, err = fmt.Printf(GlobalHelp, logdir)
			if err != nil {
				return err
			}

			return nil
		},
	}

	root.Flags().BoolVarP(&shouldPrintVersion, "version", "V", false, "prints version")

	root.AddCommand(config)
	root.AddCommand(version())
	root.AddCommand(initialize())
	root.AddCommand(execute())
	root.AddCommand(finalize())
	root.AddCommand(revert())
	root.AddCommand(restartServices)
	root.AddCommand(killServices)
	root.AddCommand(Agent())
	root.AddCommand(Hub())

	subConfigShow := createConfigShowSubcommand()
	config.AddCommand(subConfigShow)

	return addHelpToCommand(root, GlobalHelp)
}

//////////////////////////// Commands //////////////////////////////////////////

var config = &cobra.Command{
	Use:   "config",
	Short: "subcommands to set parameters for subsequent gpupgrade commands",
	Long:  "subcommands to set parameters for subsequent gpupgrade commands",
}

func createConfigShowSubcommand() *cobra.Command {
	subShow := &cobra.Command{
		Use:   "show",
		Short: "show configuration settings",
		Long:  "show configuration settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := connectToHub()
			if err != nil {
				return err
			}

			// Build a list of GetConfigRequests, one for each flag. If no flags
			// are passed, assume we want to retrieve all of them.
			var requests []*idl.GetConfigRequest
			getRequest := func(flag *pflag.Flag) {
				if flag.Name != "help" {
					requests = append(requests, &idl.GetConfigRequest{
						Name: flag.Name,
					})
				}
			}

			if cmd.Flags().NFlag() > 0 {
				cmd.Flags().Visit(getRequest)
			} else {
				cmd.Flags().VisitAll(getRequest)
			}

			// Make the requests and print every response.
			for _, request := range requests {
				resp, err := client.GetConfig(context.Background(), request)
				if err != nil {
					return err
				}

				if cmd.Flags().NFlag() == 1 {
					// Don't prefix with the setting name if the user only asked for one.
					fmt.Println(resp.Value)
				} else {
					fmt.Printf("%s - %s\n", request.Name, resp.Value)
				}
			}

			return nil
		},
	}

	subShow.Flags().Bool("id", false, "show upgrade identifier")
	subShow.Flags().Bool("source-gphome", false, "show path for the source Greenplum installation")
	subShow.Flags().Bool("target-gphome", false, "show path for the target Greenplum installation")
	subShow.Flags().Bool("target-datadir", false, "show temporary data directory for target gpdb cluster")

	return subShow
}

func version() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Version of gpupgrade",
		Long:  `Version of gpupgrade`,
		Run: func(cmd *cobra.Command, args []string) {
			printVersion()
		},
	}
}

var restartServices = &cobra.Command{
	Use:   "restart-services",
	Short: "restarts hub/agents that are not currently running",
	Long:  "restarts hub/agents that are not currently running",
	RunE: func(cmd *cobra.Command, args []string) error {
		running, err := commanders.IsHubRunning()
		if err != nil {
			return xerrors.Errorf("failed to determine if there is a hub running: %w", err)
		}

		if !running {
			err = commanders.StartHub()
			if err != nil {
				return err
			}
			fmt.Println("Restarted hub")
		}

		client, err := connectToHub()
		if err != nil {
			return err
		}

		reply, err := client.RestartAgents(context.Background(), &idl.RestartAgentsRequest{})
		if err != nil {
			return xerrors.Errorf("restarting agents: %w", err)
		}

		for _, host := range reply.GetAgentHosts() {
			fmt.Printf("Restarted agent on: %s\n", host)
		}

		return nil
	},
}

var killServices = &cobra.Command{
	Use:   "kill-services",
	Short: "Abruptly stops the hub and agents that are currently running.",
	Long: "Abruptly stops the hub and agents that are currently running.\n" +
		"Return if no hub is running, which may leave spurious agents running.",
	RunE: func(cmd *cobra.Command, args []string) error {
		running, err := commanders.IsHubRunning()
		if err != nil {
			return xerrors.Errorf("failed to determine if there is a hub running: %w", err)
		}

		if !running {
			// FIXME: Returning early if the hub is not running, means that we
			//  cannot kill spurious agents.
			// We cannot simply start the hub in order to kill spurious agents
			// since this requires initialize to have been run and the source
			// cluster config to exist. The main use case for kill-services is
			// at the start of BATS testing where we do not want to make any
			// assumption about the state of the cluster or environment.
			return nil
		}

		return stopHubAndAgents(true)
	},
}

func stopHubAndAgents(tryDefaultPort bool) error {
	client, err := connectToHubOnPort(getHubPort(tryDefaultPort))
	if err != nil {
		return err
	}

	_, err = client.StopServices(context.Background(), &idl.StopServicesRequest{})
	if err != nil {
		errCode := grpcStatus.Code(err)
		errMsg := grpcStatus.Convert(err).Message()
		// XXX: "transport is closing" is not documented but is needed to uniquely interpret codes.Unavailable
		// https://github.com/grpc/grpc/blob/v1.24.0/doc/statuscodes.md
		if errCode != codes.Unavailable || errMsg != "transport is closing" {
			return err
		}
	}
	return nil
}

//////////////////////////// Helpers ///////////////////////////////////////////

// calls connectToHubOnPort() using the port defined in the configuration file
func connectToHub() (idl.CliToHubClient, error) {
	return connectToHubOnPort(getHubPort(false))
}

// connectToHubOnPort() performs a blocking connection to the hub based on the
// passed in port, and returns a CliToHubClient which wraps the resulting gRPC channel.
// Any errors result in a call to os.Exit(1).
func connectToHubOnPort(port int) (idl.CliToHubClient, error) {
	// Set up our timeout.
	ctx, cancel := context.WithTimeout(context.Background(), connTimeout())
	defer cancel()

	// Attempt a connection.
	address := "localhost:" + strconv.Itoa(port)
	conn, err := grpc.DialContext(ctx, address, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		// Print a nicer error message if we can't connect to the hub.
		if ctx.Err() == context.DeadlineExceeded {
			gplog.Error("could not connect to the upgrade hub (did you run 'gpupgrade initialize'?)")
		}
		return nil, xerrors.Errorf("connecting to hub on port %d: %w", port, err)
	}

	return idl.NewCliToHubClient(conn), nil
}

// connTimeout retrieves the GPUPGRADE_CONNECTION_TIMEOUT environment variable,
// interprets it as a (possibly fractional) number of seconds, and converts it
// into a Duration. The default is one second if the envvar is unset or
// unreadable.
//
// TODO: should we make this a global --option instead?
func connTimeout() time.Duration {
	const defaultDuration = time.Second

	seconds, ok := os.LookupEnv("GPUPGRADE_CONNECTION_TIMEOUT")
	if !ok {
		return defaultDuration
	}

	duration, err := strconv.ParseFloat(seconds, 64)
	if err != nil {
		gplog.Warn(`GPUPGRADE_CONNECTION_TIMEOUT of "%s" is invalid (%s); using default of one second`,
			seconds, err)
		return defaultDuration
	}

	return time.Duration(duration * float64(time.Second))
}

// This reads the hub's persisted configuration for the current
// port.  If tryDefault is true and the configuration file does not exist,
// it will use the default port.  This might be the case if the hub is
// still running, even though the state directory, which contains the
// hub's persistent configuration, has been deleted.
// Any errors result in an os.Exit(1).
// NOTE: This overloads the hub's persisted configuration with that of the
// CLI when ideally these would be separate.
func getHubPort(tryDefault bool) int {
	conf := &hub.Config{}
	err := hub.LoadConfig(conf, upgrade.GetConfigFile())

	var pathError *os.PathError
	if tryDefault && xerrors.As(err, &pathError) {
		conf.Port = upgrade.DefaultHubPort
	} else if err != nil {
		gplog.Error("failed to retrieve hub port due to %v", err)
		os.Exit(1)
	}

	return conf.Port
}
