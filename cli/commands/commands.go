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
	"strings"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func BuildRootCommand() *cobra.Command {

	// TODO: if called without a subcommand, the cli prints a help message with timestamp.  Remove the timestamp.
	root := &cobra.Command{Use: "gpupgrade"}

	root.AddCommand(config, version)
	root.AddCommand(initialize())
	root.AddCommand(execute())
	root.AddCommand(finalize)
	root.AddCommand(restartServices)
	root.AddCommand(killServices)
	root.AddCommand(hub.Command())
	root.AddCommand(agent.Command())

	subConfigSet := createConfigSetSubcommand()
	subConfigShow := createConfigShowSubcommand()
	config.AddCommand(subConfigSet, subConfigShow)

	return root
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

// connectToHub() performs a blocking connection to the hub, and returns a
// CliToHubClient which wraps the resulting gRPC channel. Any errors result in
// an os.Exit(1).
func connectToHub() idl.CliToHubClient {
	upgradePort := os.Getenv("GPUPGRADE_HUB_PORT")
	if upgradePort == "" {
		upgradePort = "7527"
	}

	hubAddr := "localhost:" + upgradePort

	// Set up our timeout.
	ctx, cancel := context.WithTimeout(context.Background(), connTimeout())
	defer cancel()

	// Attempt a connection.
	conn, err := grpc.DialContext(ctx, hubAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		// Print a nicer error message if we can't connect to the hub.
		if ctx.Err() == context.DeadlineExceeded {
			gplog.Error("could not connect to the upgrade hub (did you run 'gpupgrade initialize'?)")
		} else {
			gplog.Error(err.Error())
		}
		os.Exit(1)
	}

	return idl.NewCliToHubClient(conn)
}

//////////////////////////////////////// CONFIG and its subcommands
var config = &cobra.Command{
	Use:   "config",
	Short: "subcommands to set parameters for subsequent gpupgrade commands",
	Long:  "subcommands to set parameters for subsequent gpupgrade commands",
}

func createConfigSetSubcommand() *cobra.Command {
	subSet := &cobra.Command{
		Use:   "set",
		Short: "set an upgrade parameter",
		Long:  "set an upgrade parameter",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().NFlag() == 0 {
				return errors.New("the set command requires at least one flag to be specified")
			}

			client := connectToHub()

			var requests []*idl.SetConfigRequest
			cmd.Flags().Visit(func(flag *pflag.Flag) {
				requests = append(requests, &idl.SetConfigRequest{
					Name:  flag.Name,
					Value: flag.Value.String(),
				})
			})

			for _, request := range requests {
				_, err := client.SetConfig(context.Background(), request)
				if err != nil {
					return err
				}
				gplog.Info("Successfully set %s to %s", request.Name, request.Value)
			}

			return nil
		},
	}

	subSet.Flags().String("old-bindir", "", "install directory for old gpdb version")
	subSet.Flags().String("new-bindir", "", "install directory for new gpdb version")

	return subSet
}

func createConfigShowSubcommand() *cobra.Command {
	subShow := &cobra.Command{
		Use:   "show",
		Short: "show configuration settings",
		Long:  "show configuration settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := connectToHub()

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

	subShow.Flags().Bool("old-bindir", false, "show install directory for old gpdb version")
	subShow.Flags().Bool("new-bindir", false, "show install directory for new gpdb version")

	return subShow
}

//////////////////////////////////////// VERSION
var version = &cobra.Command{
	Use:   "version",
	Short: "Version of gpupgrade",
	Long:  `Version of gpupgrade`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(utils.VersionString("gpupgrade"))
	},
}

//
// Upgrade Steps
//

func initialize() *cobra.Command {
	var oldBinDir, newBinDir string
	var oldPort int
	var diskFreeRatio float64
	var stopBeforeClusterCreation bool
	var verbose bool
	var ports string

	subInit := &cobra.Command{
		Use:   "initialize",
		Short: "prepare the system for upgrade",
		Long: `
Runs through pre-upgrade checks and prepares the old and new clusters for upgrade.
This step can be reverted.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if diskFreeRatio < 0.0 || diskFreeRatio > 1.0 {
				// Match Cobra's option-error format.
				return fmt.Errorf(
					`invalid argument %g for "--disk-free-ratio" flag: value must be between 0.0 and 1.0`,
					diskFreeRatio,
				)
			}

			ports, err := parsePorts(ports)
			if err != nil {
				return err
			}

			// If we got here, the args are okay and the user doesn't need a usage
			// dump on failure.
			cmd.SilenceUsage = true

			fmt.Println()
			fmt.Println("Initialization in progress.")
			fmt.Println()

			err = commanders.CreateStateDir()
			if err != nil {
				return errors.Wrap(err, "creating state directory")
			}

			err = commanders.CreateInitialClusterConfigs(oldBinDir, newBinDir)
			if err != nil {
				return errors.Wrap(err, "creating initial cluster configs")
			}

			running, err := commanders.IsHubRunning()
			if err != nil {
				gplog.Error("failed to determine if hub already running")
				return err
			}
			if running {
				gplog.Error("gpupgrade hub process already running")
				return errors.New("gpupgrade hub process already running")
			}

			err = commanders.StartHub()
			if err != nil {
				return errors.Wrap(err, "starting hub")
			}

			client := connectToHub()

			err = commanders.Initialize(client, oldBinDir, newBinDir, oldPort, verbose)
			if err != nil {
				return errors.Wrap(err, "initializing hub")
			}

			err = commanders.RunPreChecks(client, diskFreeRatio)
			if err != nil {
				return err
			}

			if stopBeforeClusterCreation {
				return nil
			}

			err = commanders.InitializeCreateCluster(client, verbose, ports)
			if err != nil {
				return errors.Wrap(err, "initializing cluster")
			}

			fmt.Println(`
Run "gpupgrade execute" on the command line to proceed with the upgrade.

After upgrading, you will need to finalize.

If you would like to return the cluster to its original state, run
"gpupgrade revert" on the command line.`)
			return nil
		},
	}

	subInit.PersistentFlags().StringVar(&oldBinDir, "old-bindir", "", "install directory for old gpdb version")
	subInit.MarkPersistentFlagRequired("old-bindir")
	subInit.PersistentFlags().StringVar(&newBinDir, "new-bindir", "", "install directory for new gpdb version")
	subInit.MarkPersistentFlagRequired("new-bindir")
	subInit.PersistentFlags().IntVar(&oldPort, "old-port", 0, "master port for old gpdb cluster")
	subInit.MarkPersistentFlagRequired("old-port")
	subInit.PersistentFlags().BoolVar(&stopBeforeClusterCreation, "stop-before-cluster-creation", false, "only run up to pre-init")
	subInit.PersistentFlags().MarkHidden("stop-before-cluster-creation")
	subInit.PersistentFlags().Float64Var(&diskFreeRatio, "disk-free-ratio", 0.60, "percentage of disk space that must be available (from 0.0 - 1.0)")
	subInit.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "print the output stream from all substeps")
	subInit.Flags().StringVar(&ports, "ports", "", "set of ports to use when initializing the new cluster")

	return subInit
}

func execute() *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "execute",
		Short: "executes the upgrade",
		Long: `
Upgrades the master and primary segments over to the new cluster.
This step can be reverted.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			client := connectToHub()
			return commanders.Execute(client, verbose)
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print the output stream from all substeps")

	return cmd
}

var finalize = &cobra.Command{
	Use:   "finalize",
	Short: "finalizes the cluster after upgrade execution",
	Long: `
Updates the port of the new cluster.
This step can not be reverted.
`,
	Run: func(cmd *cobra.Command, args []string) {
		client := connectToHub()
		err := commanders.Finalize(client)
		if err != nil {
			gplog.Error(err.Error())
			os.Exit(1)
		}
	},
}

func parsePorts(val string) ([]uint32, error) {
	var ports []uint32

	if val == "" {
		return ports, nil
	}

	for _, p := range strings.Split(val, ",") {
		parts := strings.Split(p, "-")
		switch {
		case len(parts) == 2: // this is a range
			low, err := strconv.ParseUint(parts[0], 10, 16)
			if err != nil {
				return nil, xerrors.Errorf("failed to parse port range %s", p)
			}

			high, err := strconv.ParseUint(parts[1], 10, 16)
			if err != nil {
				return nil, xerrors.Errorf("failed to parse port range %s", p)
			}

			if low > high {
				return nil, xerrors.Errorf("invalid port range %s", p)
			}

			for i := low; i <= high; i++ {
				ports = append(ports, uint32(i))
			}

		default: // single port
			port, err := strconv.ParseUint(p, 10, 16)
			if err != nil {
				return nil, xerrors.Errorf("failed to parse port %s", p)
			}

			ports = append(ports, uint32(port))
		}
	}

	return ports, nil
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

		reply, err := connectToHub().RestartAgents(context.Background(), &idl.RestartAgentsRequest{})
		for _, host := range reply.GetAgentHosts() {
			fmt.Printf("Restarted agent on: %s\n", host)
		}

		if err != nil {
			return xerrors.Errorf("failed to start all agents: %w", err)
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

		_, err = connectToHub().StopServices(context.Background(), &idl.StopServicesRequest{})
		if err != nil {
			errCode := grpcStatus.Code(err)
			errMsg := grpcStatus.Convert(err).Message()
			// XXX: "transport is closing" is not documented but is needed to uniquely interpret codes.Unavailable
			// https://github.com/grpc/grpc/blob/v1.24.0/doc/statuscodes.md
			if errCode != codes.Unavailable || errMsg != "transport is closing" {
				return err
			}
			return nil
		}

		return nil
	},
}
