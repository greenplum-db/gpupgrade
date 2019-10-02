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

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"
)

func BuildRootCommand() *cobra.Command {

	// TODO: if called without a subcommand, the cli prints a help message with timestamp.  Remove the timestamp.
	root := &cobra.Command{Use: "gpupgrade"}

	root.AddCommand(config, status, check, version)
	root.AddCommand(initialize())
	root.AddCommand(execute)
	root.AddCommand(subUpgradeReconfigurePorts)

	subConfigSet := createConfigSetSubcommand()
	subConfigShow := createConfigShowSubcommand()
	config.AddCommand(subConfigSet, subConfigShow)

	status.AddCommand(subStatusUpgrade, subStatusConversion)

	check.AddCommand(subCheckObjectCount, subCheckDiskSpace)

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

//////////////////////////////////////// CHECK and its subcommands
var check = &cobra.Command{
	Use:   "check",
	Short: "collects information and validates the target Greenplum installation can be upgraded",
	Long:  `collects information and validates the target Greenplum installation can be upgraded`,
}

var subCheckDiskSpace = &cobra.Command{
	Use:     "disk-space",
	Short:   "check that disk space usage is less than 80% on all segments",
	Long:    "check that disk space usage is less than 80% on all segments",
	Aliases: []string{"du"},
	RunE: func(cmd *cobra.Command, args []string) error {
		client := connectToHub()
		return commanders.NewDiskSpaceChecker(client).Execute()
	},
}
var subCheckObjectCount = &cobra.Command{
	Use:     "object-count",
	Short:   "count database objects and numeric objects",
	Long:    "count database objects and numeric objects",
	Aliases: []string{"oc"},
	RunE: func(cmd *cobra.Command, args []string) error {
		client := connectToHub()
		return commanders.NewObjectCountChecker(client).Execute()
	},
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

//////////////////////////////////////// STATUS and its subcommands
var status = &cobra.Command{
	Use:   "status",
	Short: "subcommands to show the status of a gpupgrade",
	Long:  "subcommands to show the status of a gpupgrade",
}

var subStatusConversion = &cobra.Command{
	Use:   "conversion",
	Short: "the status of the conversion",
	Long:  "the status of the conversion",
	Run: func(cmd *cobra.Command, args []string) {
		client := connectToHub()
		reporter := commanders.NewReporter(client)
		err := reporter.OverallConversionStatus()
		if err != nil {
			gplog.Error(err.Error())
			os.Exit(1)
		}
	},
}
var subStatusUpgrade = &cobra.Command{
	Use:   "upgrade",
	Short: "the status of the upgrade",
	Long:  "the status of the upgrade",
	Run: func(cmd *cobra.Command, args []string) {
		client := connectToHub()
		reporter := commanders.NewReporter(client)
		err := reporter.OverallUpgradeStatus()
		if err != nil {
			gplog.Error(err.Error())
			os.Exit(1)
		}
	},
}

//////////////////////////////////////// UPGRADE and its subcommands
var subUpgradeReconfigurePorts = &cobra.Command{
	Use:   "reconfigure-ports",
	Short: "Set master port on upgraded cluster to the value from the older cluster",
	Long:  `Set master port on upgraded cluster to the value from the older cluster`,
	Run: func(cmd *cobra.Command, args []string) {
		client := connectToHub()
		err := commanders.NewUpgrader(client).ReconfigurePorts()
		if err != nil {
			gplog.Error(err.Error())
			os.Exit(1)
		}
	},
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

//////////////////////////////////////// Initialize
func initialize() *cobra.Command {
	var oldBinDir, newBinDir string
	var oldPort int

	subInit := &cobra.Command{
		Use:   "initialize",
		Short: "prepare the system for upgrade",
		Long:  `prepare the system for upgrade`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// If we got here, the args are okay and the user doesn't need a usage
			// dump on failure.
			cmd.SilenceUsage = true

			err := commanders.CreateStateDirAndClusterConfigs(oldBinDir, newBinDir)
			if err != nil {
				return errors.Wrap(err, "tried to create state directory")
			}

			err = commanders.StartHub()
			if err != nil {
				return errors.Wrap(err, "starting hub")
			}

			client := connectToHub()
			err = commanders.Initialize(client, oldBinDir, newBinDir, oldPort)
			if err != nil {
				return errors.Wrap(err, "initializing hub")
			}

			// TODO: how do we rollback here?
			return commanders.NewVersionChecker(client).Execute()

		},
	}

	subInit.PersistentFlags().StringVar(&oldBinDir, "old-bindir", "", "install directory for old gpdb version")
	subInit.MarkPersistentFlagRequired("old-bindir")
	subInit.PersistentFlags().StringVar(&newBinDir, "new-bindir", "", "install directory for new gpdb version")
	subInit.MarkPersistentFlagRequired("new-bindir")
	subInit.PersistentFlags().IntVar(&oldPort, "old-port", 0, "master port for old gpdb cluster")
	subInit.MarkPersistentFlagRequired("old-port")

	return subInit
}

//////////////////////////////////////// Execute
var execute = &cobra.Command{
	Use:   "execute",
	Short: "executes the upgrade",
	Long:  "Executes the upgrade",
	Run: func(cmd *cobra.Command, args []string) {
		client := connectToHub()
		err := commanders.Execute(client)
		if err != nil {
			gplog.Error(err.Error())
			os.Exit(1)
		}
	},
}
