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
	"path/filepath"
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

	"github.com/greenplum-db/gpupgrade/cli"
	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

var (
	InitializeWarningMessage = `
WARNING
_______
The source cluster does not have %s.
After "gpupgrade execute" has been run, there will be no way to
return the cluster to its original state using "gpupgrade revert".

If you do no already have a backup, we strongly recommend that
you run "gpupgrade revert" now and take a backup of the cluster.
`
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
	root.AddCommand(revert())
	root.AddCommand(restartServices)
	root.AddCommand(killServices)
	root.AddCommand(Agent())
	root.AddCommand(Hub())

	subConfigShow := createConfigShowSubcommand()
	config.AddCommand(subConfigShow)

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

//////////////////////////////////////// CONFIG and its subcommands
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
	var file string
	var automatic bool
	var sourceGPHome, targetGPHome string
	var sourcePort int
	var hubPort int
	var agentPort int
	var diskFreeRatio float64
	var stopBeforeClusterCreation bool
	var verbose bool
	var skipVersionCheck bool
	var ports string
	var mode string

	subInit := &cobra.Command{
		Use:   "initialize",
		Short: "prepare the system for upgrade",
		Long:  InitializeHelp,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// mark the required flags when the file flag is not set
			if !cmd.Flag("file").Changed {
				cmd.MarkFlagRequired("source-gphome")      //nolint
				cmd.MarkFlagRequired("target-gphome")      //nolint
				cmd.MarkFlagRequired("source-master-port") //nolint
			}

			// If the file flag is set check that no other flags are specified
			// other than verbose and automatic.
			if cmd.Flag("file").Changed {
				var err error
				cmd.Flags().Visit(func(flag *pflag.Flag) {
					if flag.Name != "file" && flag.Name != "verbose" && flag.Name != "automatic" {
						err = errors.New("The file flag cannot be used with any other flag.")
					}
				})
				return err
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if cmd.Flag("file").Changed {
				configFile, err := os.Open(file)
				if err != nil {
					return err
				}
				defer func() {
					if cErr := configFile.Close(); cErr != nil {
						err = errorlist.Append(err, cErr)
					}
				}()

				flags, err := ParseConfig(configFile)
				if err != nil {
					return xerrors.Errorf("in file %q: %w", file, err)
				}

				err = addFlags(cmd, flags)
				if err != nil {
					return err
				}
			}

			linkMode, err := isLinkMode(mode)
			if err != nil {
				return err
			}

			// if diskFreeRatio is not explicitly set, use defaults
			if !cmd.Flag("disk-free-ratio").Changed {
				if linkMode {
					diskFreeRatio = 0.2
				} else {
					diskFreeRatio = 0.6
				}
			}

			if diskFreeRatio < 0.0 || diskFreeRatio > 1.0 {
				// Match Cobra's option-error format.
				return fmt.Errorf(
					`invalid argument %g for "--disk-free-ratio" flag: value must be between 0.0 and 1.0`,
					diskFreeRatio,
				)
			}

			parsedPorts, err := parsePorts(ports)
			if err != nil {
				return err
			}

			logdir, err := utils.GetLogDir()
			if err != nil {
				return err
			}

			configPath, err := filepath.Abs(file)
			if err != nil {
				return err
			}

			// If we got here, the args are okay and the user doesn't need a usage
			// dump on failure.
			cmd.SilenceUsage = true

			// Create the state directory outside the step framework to ensure
			// we can write to the status file. The step framework assumes valid
			// working state directory.
			err = commanders.CreateStateDir()
			if err != nil {
				nextActions := fmt.Sprintf(`Please address the above issue and run "gpupgrade %s" again.`, strings.ToLower(idl.Step_INITIALIZE.String()))
				return cli.NewNextActions(err, nextActions)
			}

			confirmationText := fmt.Sprintf(initializeConfirmationText, logdir, configPath, sourceGPHome, targetGPHome,
				mode, diskFreeRatio, sourcePort, ports, hubPort, agentPort)

			st, err := commanders.NewStep(idl.Step_INITIALIZE,
				&step.BufferedStreams{},
				verbose,
				automatic,
				confirmationText,
			)
			if err != nil {
				if errors.Is(err, step.UserCanceled) {
					// If user cancels don't return an error to main to avoid
					// printing "Error:".
					return nil
				}
				return err
			}

			st.RunInternalSubstep(func() error {
				if skipVersionCheck {
					return nil
				}

				err := cli.ValidateVersions(sourceGPHome, targetGPHome)
				if err != nil {
					nextActions := fmt.Sprintf(`Please address the above issue and run "gpupgrade %s" again.`, strings.ToLower(idl.Step_INITIALIZE.String()))
					return cli.NewNextActions(err, nextActions)
				}

				return nil
			})

			st.RunInternalSubstep(func() error {
				return commanders.CreateInitialClusterConfigs(hubPort)
			})

			st.RunCLISubstep(idl.Substep_START_HUB, func(streams step.OutStreams) error {
				return commanders.StartHub()
			})

			var client idl.CliToHubClient
			st.RunHubSubstep(func(streams step.OutStreams) error {
				client, err = connectToHub()
				if err != nil {
					return err
				}

				request := &idl.InitializeRequest{
					AgentPort:    int32(agentPort),
					SourceGPHome: filepath.Clean(sourceGPHome),
					TargetGPHome: filepath.Clean(targetGPHome),
					SourcePort:   int32(sourcePort),
					UseLinkMode:  linkMode,
					Ports:        parsedPorts,
				}
				err = commanders.Initialize(client, request, verbose)
				if err != nil {
					return xerrors.Errorf("initialize hub: %w", err)
				}

				return nil
			})

			st.RunCLISubstep(idl.Substep_CHECK_DISK_SPACE, func(streams step.OutStreams) error {
				return commanders.CheckDiskSpace(client, diskFreeRatio)
			})

			var response idl.InitializeResponse
			st.RunHubSubstep(func(streams step.OutStreams) error {
				if stopBeforeClusterCreation {
					return step.Skip
				}

				response, err = commanders.InitializeCreateCluster(client, verbose)
				if err != nil {
					return xerrors.Errorf("initialize create cluster: %w", err)
				}

				return nil
			})

			warningMessage := InitializeWarningMessageIfAny(response)

			return st.Complete(fmt.Sprintf(`
Initialize completed successfully.
%s
NEXT ACTIONS
------------
To proceed with the upgrade, run "gpupgrade execute"
followed by "gpupgrade finalize".

To return the cluster to its original state, run "gpupgrade revert".`,
				warningMessage))
		},
	}
	subInit.Flags().StringVarP(&file, "file", "f", "", "the configuration file to use")
	subInit.Flags().BoolVarP(&automatic, "automatic", "a", false, "do not prompt for confirmation to proceed")
	subInit.Flags().StringVar(&sourceGPHome, "source-gphome", "", "path for the source Greenplum installation")
	subInit.Flags().StringVar(&targetGPHome, "target-gphome", "", "path for the target Greenplum installation")
	subInit.Flags().IntVar(&sourcePort, "source-master-port", 5432, "master port for source gpdb cluster")
	subInit.Flags().IntVar(&hubPort, "hub-port", upgrade.DefaultHubPort, "the port gpupgrade hub uses to listen for commands on")
	subInit.Flags().IntVar(&agentPort, "agent-port", upgrade.DefaultAgentPort, "the port gpupgrade agent uses to listen for commands on")
	subInit.Flags().BoolVar(&stopBeforeClusterCreation, "stop-before-cluster-creation", false, "only run up to pre-init")
	subInit.Flags().MarkHidden("stop-before-cluster-creation") //nolint
	subInit.Flags().Float64Var(&diskFreeRatio, "disk-free-ratio", 0.60, "percentage of disk space that must be available (from 0.0 - 1.0)")
	subInit.Flags().BoolVarP(&verbose, "verbose", "v", false, "print the output stream from all substeps")
	subInit.Flags().StringVar(&ports, "temp-port-range", "50432-65535", "set of ports to use when initializing the target cluster")
	subInit.Flags().StringVar(&mode, "mode", "copy", "performs upgrade in either copy or link mode. Default is copy.")
	subInit.Flags().BoolVar(&skipVersionCheck, "skip-version-check", false, "disable source and target version check")
	subInit.Flags().MarkHidden("skip-version-check") //nolint
	return addHelpToCommand(subInit, InitializeHelp)
}

func execute() *cobra.Command {
	var verbose bool
	var automatic bool

	cmd := &cobra.Command{
		Use:   "execute",
		Short: "executes the upgrade",
		Long:  ExecuteHelp,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			cmd.SilenceUsage = true
			var response idl.ExecuteResponse

			logdir, err := utils.GetLogDir()
			if err != nil {
				return err
			}

			confirmationText := fmt.Sprintf(executeConfirmationText, logdir)

			st, err := commanders.NewStep(idl.Step_EXECUTE,
				&step.BufferedStreams{},
				verbose,
				automatic,
				confirmationText,
			)
			if err != nil {
				if errors.Is(err, step.UserCanceled) {
					// If user cancels don't return an error to main to avoid
					// printing "Error:".
					return nil
				}
				return err
			}

			st.RunHubSubstep(func(streams step.OutStreams) error {
				client, err := connectToHub()
				if err != nil {
					return err
				}

				response, err = commanders.Execute(client, verbose)
				if err != nil {
					return err
				}

				return nil
			})

			return st.Complete(fmt.Sprintf(`
Execute completed successfully.

The target cluster is now running. You may now run queries against the target 
database and perform any other validation desired prior to finalizing your upgrade.
PGPORT: %d
MASTER_DATA_DIRECTORY: %s

WARNING: If any queries modify the target database prior to gpupgrade finalize, 
it will be inconsistent with the source database. 

NEXT ACTIONS
------------
If you are satisfied with the state of the cluster, run "gpupgrade finalize" 
to proceed with the upgrade.

To return the cluster to its original state, run "gpupgrade revert".`,
				response.GetTarget().GetPort(), response.GetTarget().GetMasterDataDirectory()))
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print the output stream from all substeps")
	cmd.Flags().BoolVarP(&automatic, "automatic", "a", false, "do not prompt for confirmation to proceed")
	cmd.Flags().MarkHidden("automatic") //nolint

	return addHelpToCommand(cmd, ExecuteHelp)
}

func finalize() *cobra.Command {
	var verbose bool
	var automatic bool

	cmd := &cobra.Command{
		Use:   "finalize",
		Short: "finalizes the cluster after upgrade execution",
		Long:  FinalizeHelp,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			var response idl.FinalizeResponse

			logdir, err := utils.GetLogDir()
			if err != nil {
				return err
			}

			confirmationText := fmt.Sprintf(finalizeConfirmationText, logdir)

			st, err := commanders.NewStep(idl.Step_FINALIZE,
				&step.BufferedStreams{},
				verbose,
				automatic,
				confirmationText,
			)
			if err != nil {
				if errors.Is(err, step.UserCanceled) {
					// If user cancels don't return an error to main to avoid
					// printing "Error:".
					return nil
				}
				return err
			}

			st.RunHubSubstep(func(streams step.OutStreams) error {
				client, err := connectToHub()
				if err != nil {
					return err
				}

				response, err = commanders.Finalize(client, verbose)
				if err != nil {
					return err
				}

				return nil
			})

			return st.Complete(fmt.Sprintf(`
Finalize completed successfully.

The target cluster is now ready to use, running Greenplum %s.
PGPORT: %d
MASTER_DATA_DIRECTORY: %s

NEXT ACTIONS
------------
Run the “complete” data migration scripts, and recreate any additional tables,
indexes, and roles that were dropped or altered to resolve migration issues.`,
				response.GetTargetVersion(), response.GetTarget().GetPort(), response.GetTarget().GetMasterDataDirectory()))
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print the output stream from all substeps")
	cmd.Flags().BoolVarP(&automatic, "automatic", "a", false, "do not prompt for confirmation to proceed")
	cmd.Flags().MarkHidden("automatic") //nolint

	return addHelpToCommand(cmd, FinalizeHelp)
}

func revert() *cobra.Command {
	var verbose bool
	var automatic bool

	cmd := &cobra.Command{
		Use:   "revert",
		Short: "reverts the upgrade and returns the cluster to its original state",
		Long:  RevertHelp,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			var response idl.RevertResponse

			logdir, err := utils.GetLogDir()
			if err != nil {
				return err
			}

			confirmationText := fmt.Sprintf(revertConfirmationText, logdir)

			st, err := commanders.NewStep(idl.Step_REVERT,
				&step.BufferedStreams{},
				verbose,
				automatic,
				confirmationText,
			)
			if err != nil {
				if errors.Is(err, step.UserCanceled) {
					// If user cancels don't return an error to main to avoid
					// printing "Error:".
					return nil
				}
				return err
			}

			st.RunHubSubstep(func(streams step.OutStreams) error {
				client, err := connectToHub()
				if err != nil {
					return err
				}

				response, err = commanders.Revert(client, verbose)
				if err != nil {
					return err
				}

				return nil
			})

			st.RunCLISubstep(idl.Substep_STOP_HUB_AND_AGENTS, func(streams step.OutStreams) error {
				return stopHubAndAgents(false)
			})

			st.RunCLISubstep(idl.Substep_DELETE_MASTER_STATEDIR, func(streams step.OutStreams) error {
				// Removing the state directory removes the step status file.
				// Disable the store so the step framework does not try to write
				// to a non-existent status file.
				st.DisableStore()
				return upgrade.DeleteDirectories([]string{utils.GetStateDir()}, upgrade.StateDirectoryFiles, streams)
			})

			return st.Complete(fmt.Sprintf(`
Revert completed successfully.

The source cluster is now running version %s.
PGPORT: %d
MASTER_DATA_DIRECTORY: %s

The gpupgrade logs can be found on the master and segment hosts in
%s

NEXT ACTIONS
------------
To use the reverted cluster, run the “revert” data migration scripts, and
recreate any additional tables, indexes, and roles that were dropped or
altered to resolve migration issues.

To restart the upgrade, run "gpupgrade initialize" again.`,
				response.GetSourceVersion(), response.GetSource().GetPort(), response.GetSource().GetMasterDataDirectory(), response.GetLogArchiveDirectory()))
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print the output stream from all substeps")
	cmd.Flags().BoolVarP(&automatic, "automatic", "a", false, "do not prompt for confirmation to proceed")
	cmd.Flags().MarkHidden("automatic") //nolint

	return addHelpToCommand(cmd, RevertHelp)
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

// isLinkMode parses the mode flag returning an error if it is not copy or link.
// It returns true if mode is link.
func isLinkMode(input string) (bool, error) {
	choices := []string{"copy", "link"}

	mode := strings.ToLower(strings.TrimSpace(input))
	for _, choice := range choices {
		if mode == choice {
			return mode == "link", nil
		}
	}

	return false, fmt.Errorf("Invalid input %q. Please specify either %s.", input, strings.Join(choices, " or "))
}

func addFlags(cmd *cobra.Command, flags map[string]string) error {
	for name, value := range flags {
		flag := cmd.Flag(name)
		if flag == nil {
			var names []string
			cmd.Flags().VisitAll(func(flag *pflag.Flag) {
				names = append(names, flag.Name)
			})
			return xerrors.Errorf("The configuration parameter %q was not found in the list of supported parameters: %s.", name, strings.Join(names, ", "))
		}

		err := flag.Value.Set(value)
		if err != nil {
			return xerrors.Errorf("set %q to %q: %w", name, value, err)
		}

		cmd.Flag(name).Changed = true
	}

	return nil
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

func InitializeWarningMessageIfAny(response idl.InitializeResponse) string {
	message := ""
	if !response.GetHasStandby() && !response.GetHasMirrors() {
		message = "standby and mirror segments"
	} else if !response.GetHasMirrors() {
		message = "mirror segments"
	} else if !response.GetHasStandby() {
		message = "standby"
	}

	if message != "" {
		return fmt.Sprintf(InitializeWarningMessage, message)
	}

	return message
}
