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

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
)

func BuildRootCommand() *cobra.Command {
	// TODO: if called without a subcommand, the cli prints a help message with timestamp.  Remove the timestamp.
	var shouldPrintVersion bool

	root := &cobra.Command{
		Use: "gpupgrade",
		Run: func(cmd *cobra.Command, args []string) {
			if shouldPrintVersion {
				printVersion()
				return
			}

			fmt.Print(GlobalHelp)
		},
	}

	root.Flags().BoolVarP(&shouldPrintVersion, "version", "V", false, "prints version")

	root.AddCommand(config)
	root.AddCommand(version())
	root.AddCommand(initialize())
	root.AddCommand(execute())
	root.AddCommand(finalize())
	root.AddCommand(restartServices)
	root.AddCommand(killServices)
	root.AddCommand(Agent())
	root.AddCommand(Hub())

	subConfigSet := createConfigSetSubcommand()
	subConfigShow := createConfigShowSubcommand()
	config.AddCommand(subConfigSet, subConfigShow)

	return addHelpToCommand(root, GlobalHelp)
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

	subSet.Flags().String("source-bindir", "", "install directory for old gpdb version")
	subSet.Flags().String("target-bindir", "", "install directory for new gpdb version")

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

	subShow.Flags().Bool("source-bindir", false, "show install directory for source gpdb version")
	subShow.Flags().Bool("target-bindir", false, "show install directory for target gpdb version")
	subShow.Flags().Bool("target-datadir", false, "show temporary data directory for target gpdb cluster")

	return subShow
}

//////////////////////////////////////// VERSION
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

//
// Upgrade Steps
//

func initialize() *cobra.Command {
	var sourceBinDir, targetBinDir string
	var sourcePort int
	var diskFreeRatio float64
	var stopBeforeClusterCreation bool
	var verbose bool
	var ports string
	var linkMode bool

	subInit := &cobra.Command{
		Use:   "initialize",
		Short: "prepare the system for upgrade",
		Long:  InitializeHelp,
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
			fmt.Println("Initialize in progress.")
			fmt.Println()

			err = commanders.CreateStateDir()
			if err != nil {
				return errors.Wrap(err, "creating state directory")
			}

			err = commanders.CreateInitialClusterConfigs()
			if err != nil {
				return errors.Wrap(err, "creating initial cluster configs")
			}

			err = commanders.StartHub()
			if err != nil {
				return errors.Wrap(err, "starting hub")
			}

			client := connectToHub()

			request := &idl.InitializeRequest{
				SourceBinDir: sourceBinDir,
				TargetBinDir: targetBinDir,
				SourcePort:   int32(sourcePort),
				UseLinkMode:  linkMode,
				Ports:        ports,
			}
			err = commanders.Initialize(client, request, verbose)
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

			err = commanders.InitializeCreateCluster(client, verbose)
			if err != nil {
				return errors.Wrap(err, "initializing cluster")
			}

			fmt.Println(`
Initialize completed successfully.

NEXT ACTIONS
------------
Run "gpupgrade execute" to proceed with the upgrade.

After executing, you will need to finalize.`)

			return nil
		},
	}

	subInit.Flags().StringVar(&sourceBinDir, "source-bindir", "", "install directory for source gpdb version")
	subInit.MarkFlagRequired("source-bindir")
	subInit.Flags().StringVar(&targetBinDir, "target-bindir", "", "install directory for target gpdb version")
	subInit.MarkFlagRequired("target-bindir")
	subInit.Flags().IntVar(&sourcePort, "source-master-port", 0, "master port for old gpdb cluster")
	subInit.MarkFlagRequired("source-master-port")
	subInit.Flags().BoolVar(&stopBeforeClusterCreation, "stop-before-cluster-creation", false, "only run up to pre-init")
	subInit.Flags().MarkHidden("stop-before-cluster-creation")
	subInit.Flags().Float64Var(&diskFreeRatio, "disk-free-ratio", 0.60, "percentage of disk space that must be available (from 0.0 - 1.0)")
	subInit.Flags().BoolVarP(&verbose, "verbose", "v", false, "print the output stream from all substeps")
	subInit.Flags().StringVar(&ports, "temp-port-range", "", "set of ports to use when initializing the new cluster")
	subInit.Flags().BoolVar(&linkMode, "link", false, "performs upgrade in link mode")

	return addHelpToCommand(subInit, InitializeHelp)
}

func execute() *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "execute",
		Short: "executes the upgrade",
		Long:  ExecuteHelp,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			client := connectToHub()
			return commanders.Execute(client, verbose)
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print the output stream from all substeps")

	return addHelpToCommand(cmd, ExecuteHelp)
}

func finalize() *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "finalize",
		Short: "finalizes the cluster after upgrade execution",
		Long:  FinalizeHelp,
		Run: func(cmd *cobra.Command, args []string) {
			client := connectToHub()
			err := commanders.Finalize(client, verbose)
			if err != nil {
				gplog.Error(err.Error())
				os.Exit(1)
			}
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print the output stream from all substeps")

	return addHelpToCommand(cmd, FinalizeHelp)
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

const (
	InitializeHelp = `
Runs through pre-upgrade checks and prepares the cluster for upgrade.

Initialize will carry out the following sub-steps:
 - Create directories
 - Generate upgrade configuration
 - Start gpupgrade hub process
 - Retrieve source cluster configuration
 - Start gpupgrade agent processes
 - Check disk space
 - Generate target cluster configuration
 - Create target cluster
 - Stop target cluster
 - Back up target master
 - Run pg_upgrade checks

Usage: gpupgrade initialize <flags>

Required Flags:

  --source-bindir         the path to the binary directory for the source Greenplum installation

  --target-bindir         the path to the binary directory for the target Greenplum installation

  --source-master-port    the master port for the source Greenplum installation

Optional Flags:

  -h, --help              displays help output for initialize

      --link              an alternative mode that directly upgrades the primary segments

      --temp-port-range   the set of ports to use when initializing the target cluster

  -v, --verbose           outputs detailed logs for initialize
`
	ExecuteHelp = `
Upgrades the master and primary segments to the target Greenplum version.

Execute will carry out the following sub-steps:
 - Stop source cluster
 - Upgrade master
 - Copy master catalog to primary segments
 - Upgrade primary segments
 - Start target cluster

Usage: gpupgrade execute

Optional Flags:

  -h, --help      displays help output for execute

  -v, --verbose   outputs detailed logs for execute
`
	FinalizeHelp = `
Upgrades the standby master and mirror segments to the target Greenplum version.

Finalize will carry out the following sub-steps:
- Upgrade standby master
- Upgrade mirror segments
- Stop target cluster
- Start target master
- Update target master port
- Stop target master
- Update target master postgresql.conf
- Start target cluster

Usage: gpupgrade finalize

Optional Flags:

  -h, --help      displays help output for finalize

  -v, --verbose   outputs detailed logs for finalize
`
	GlobalHelp = `
gpupgrade enables users to do an in-place cluster upgrade to the next major version.
The default mode is copy, which creates a copy of the primary segments and performs the upgrade on the copies.

Usage: gpupgrade [command] <flags> 

Required Commands: gpupgrade is a three-step process

  1. initialize   runs through pre-upgrade checks and prepares the cluster for upgrade

                  Usage: gpupgrade initialize <flags>

                  Required Flags:
                    --source-bindir        the path to the binary directory for the source Greenplum installation
                    --target-bindir        the path to the binary directory for the target Greenplum installation
                    --source-master-port   the master port for the source Greenplum installation

                  Optional Flags:
                    --link                 an alternative mode that directly upgrades the primary segments
                    --temp-port-range      the set of ports to use when initializing the target cluster

  2. execute      upgrades the master and primary segments to the target Greenplum version

  3. finalize     upgrades the standby master and mirror segments to the target Greenplum version

Optional Flags:

  -h, --help      displays help output for gpupgrade

  -v, --verbose   outputs detailed logs for gpupgrade

  -V, --version   displays the version of the current gpupgrade utility

Use "gpupgrade [command] --help" for more information about a command.
`
)

// Cobra has multiple ways to handle help text, so we want to force all of them to use the same help text
func addHelpToCommand(cmd *cobra.Command, help string) *cobra.Command {
	// Add a "-?" flag, which Cobra does not provide by default
	var savedPreRunE func(cmd *cobra.Command, args []string) error
	var savedPreRun func(cmd *cobra.Command, args []string)
	if cmd.PreRunE != nil {
		savedPreRunE = cmd.PreRunE
	} else if cmd.PreRun != nil {
		savedPreRun = cmd.PreRun
	}

	var questionHelp bool
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if questionHelp {
			fmt.Print(help)
			os.Exit(0)
		}
		if savedPreRunE != nil {
			return savedPreRunE(cmd, args)
		} else if savedPreRun != nil {
			savedPreRun(cmd, args)
		}
		return nil
	}
	cmd.Flags().BoolVarP(&questionHelp, "?", "?", false, "displays help output")

	// Override the built-in "help" subcommand
	cmd.AddCommand(&cobra.Command{
		Use:   "help",
		Short: "",
		Long:  "",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf(help)
			return nil
		},
	})
	cmd.SetUsageTemplate(help)

	// Override the built-in "-h" and "--help" flags
	cmd.SetHelpFunc(func(cmd *cobra.Command, strs []string) {
		fmt.Printf(help)
	})

	return cmd
}
