// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

const InitializeWarningMessage = `
WARNING
_______
The source cluster does not have %s.
After "gpupgrade execute" has been run, there will be no way to
return the cluster to its original state using "gpupgrade revert".

If you do not already have a backup, we strongly recommend that
you run "gpupgrade revert" now and take a backup of the cluster.
`

func initialize() *cobra.Command {
	var file string
	var nonInteractive bool
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
	var useHbaHostnames bool
	var dynamicLibraryPath string

	subInit := &cobra.Command{
		Use:   "initialize",
		Short: "prepare the system for upgrade",
		Long:  InitializeHelp,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			isAnyDevModeFlagSet := cmd.Flag("source-gphome").Changed ||
				cmd.Flag("target-gphome").Changed ||
				cmd.Flag("source-master-port").Changed

			// If no required flags are set then return help.
			if !cmd.Flag("file").Changed && !isAnyDevModeFlagSet {
				fmt.Println(Help["initialize"])
				cmd.SilenceErrors = true // silence UserCanceled error message below
				return step.UserCanceled // exit early and don't call RunE
			}

			// If the file flag is set ensure no other flags are set except
			// optionally verbose and automatic.
			if cmd.Flag("file").Changed {
				var err error
				cmd.Flags().Visit(func(flag *pflag.Flag) {
					if flag.Name != "file" && flag.Name != "verbose" && flag.Name != "automatic" {
						err = errors.New("The file flag cannot be used with any other flag except verbose and automatic.")
					}
				})
				return err
			}

			// In dev mode the file flag should not be set and ensure all dev
			// mode flags are set by marking them required.
			if !cmd.Flag("file").Changed && isAnyDevModeFlagSet {
				devModeFlags := []string{
					"source-gphome",
					"target-gphome",
					"source-master-port",
				}

				for _, f := range devModeFlags {
					cmd.MarkFlagRequired(f) //nolint
				}
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
				return err
			}

			confirmationText := fmt.Sprintf(initializeConfirmationText, logdir, configPath,
				sourcePort, sourceGPHome, targetGPHome, mode, diskFreeRatio, useHbaHostnames, dynamicLibraryPath, ports, hubPort, agentPort)

			st, err := commanders.NewStep(idl.Step_INITIALIZE,
				&step.BufferedStreams{},
				verbose,
				nonInteractive,
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

				err := greenplum.VerifyCompatibleGPDBVersions(sourceGPHome, targetGPHome)
				if err != nil {
					return err
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
					AgentPort:       int32(agentPort),
					SourceGPHome:    filepath.Clean(sourceGPHome),
					TargetGPHome:    filepath.Clean(targetGPHome),
					SourcePort:      int32(sourcePort),
					LinkMode:        linkMode,
					UseHbaHostnames: useHbaHostnames,
					Ports:           parsedPorts,
					DiskFreeRatio:   diskFreeRatio,
				}
				err = commanders.Initialize(client, request, verbose)
				if err != nil {
					return err
				}

				return nil
			})

			var response idl.InitializeResponse
			st.RunHubSubstep(func(streams step.OutStreams) error {
				if stopBeforeClusterCreation {
					return step.Skip
				}

				request := &idl.InitializeCreateClusterRequest{
					DynamicLibraryPath: dynamicLibraryPath,
				}
				response, err = commanders.InitializeCreateCluster(client, request, verbose)
				if err != nil {
					return err
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

	subInit.Flags().BoolVarP(&verbose, "verbose", "v", false, "print the output stream from all substeps")
	subInit.Flags().StringVarP(&file, "file", "f", "", "the configuration file to use")
	subInit.Flags().BoolVarP(&nonInteractive, "automatic", "a", false, "do not prompt for confirmation to proceed")
	subInit.Flags().BoolVar(&nonInteractive, "non-interactive", false, "do not prompt for confirmation to proceed")
	subInit.Flags().MarkHidden("non-interactive") //nolint
	subInit.Flags().IntVar(&sourcePort, "source-master-port", 0, "master port for source gpdb cluster")
	subInit.Flags().StringVar(&sourceGPHome, "source-gphome", "", "path for the source Greenplum installation")
	subInit.Flags().StringVar(&targetGPHome, "target-gphome", "", "path for the target Greenplum installation")
	subInit.Flags().StringVar(&mode, "mode", "copy", "performs upgrade in either copy or link mode. Default is copy.")
	subInit.Flags().Float64Var(&diskFreeRatio, "disk-free-ratio", 0.60, "percentage of disk space that must be available (from 0.0 - 1.0)")
	subInit.Flags().BoolVar(&useHbaHostnames, "use-hba-hostnames", false, "use hostnames in pg_hba.conf")
	subInit.Flags().StringVar(&dynamicLibraryPath, "dynamic-library-path", upgrade.DefaultDynamicLibraryPath, "sets the dynamic_library_path GUC to correctly find extensions installed outside their default location. Defaults to '$dynamic_library_path'.")
	subInit.Flags().StringVar(&ports, "temp-port-range", "50432-65535", "set of ports to use when initializing the target cluster")
	subInit.Flags().IntVar(&hubPort, "hub-port", upgrade.DefaultHubPort, "the port gpupgrade hub uses to listen for commands on")
	subInit.Flags().IntVar(&agentPort, "agent-port", upgrade.DefaultAgentPort, "the port gpupgrade agent uses to listen for commands on")
	subInit.Flags().BoolVar(&stopBeforeClusterCreation, "stop-before-cluster-creation", false, "only run up to pre-init")
	subInit.Flags().MarkHidden("stop-before-cluster-creation") //nolint
	subInit.Flags().BoolVar(&skipVersionCheck, "skip-version-check", false, "disable source and target version check")
	subInit.Flags().MarkHidden("skip-version-check") //nolint
	return addHelpToCommand(subInit, InitializeHelp)
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
