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
 * 			seginstall   confirms that the new software is installed on all segments
 * 			version      validate current version is upgradable
 *
 * 		Flags:
 * 			-h, --help   help for check
 *
 * 		Use "gpupgrade check [command] --help" for more information about a command.
 */

import (
	"fmt"
	"os"

	"github.com/greenplum-db/gpupgrade/cli/commanders"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func BuildRootCommand() *cobra.Command {

	// TODO: if called without a subcommand, the cli prints a help message with timestamp.  Remove the timestamp.
	root := &cobra.Command{Use: "gpupgrade"}

	root.AddCommand(prepare, config, status, check, version, upgrade)

	subPrepareInit := createPrepareInitSubcommand()
	prepare.AddCommand(subPrepareStartHub, subPrepareInitCluster, subPrepareShutdownClusters, subPrepareStartAgents,
		subPrepareInit)

	subConfigSet := createConfigSetSubcommand()
	subConfigShow := createConfigShowSubcommand()
	config.AddCommand(subConfigSet, subConfigShow)

	status.AddCommand(subStatusUpgrade, subStatusConversion)

	check.AddCommand(subCheckVersion, subCheckObjectCount, subCheckDiskSpace, subCheckConfig, subCheckSeginstall)

	upgrade.AddCommand(subUpgradeConvertMaster, subUpgradeConvertPrimaries, subUpgradeShareOids,
		subUpgradeValidateStartCluster, subUpgradeReconfigurePorts)

	return root
}

//////////////////////////////////////// CHECK and its subcommands
var check = &cobra.Command{
	Use:   "check",
	Short: "collects information and validates the target Greenplum installation can be upgraded",
	Long:  `collects information and validates the target Greenplum installation can be upgraded`,
}

var subCheckConfig = &cobra.Command{
	Use:   "config",
	Short: "gather cluster configuration",
	Long:  "gather cluster configuration",
	Run: func(cmd *cobra.Command, args []string) {
		err := commanders.NewConfigCheckerCmd()
		if err != nil {
			gplog.Error(err.Error())
			os.Exit(1)
		}
	},
}
var subCheckDiskSpace = &cobra.Command{
	Use:     "disk-space",
	Short:   "check that disk space usage is less than 80% on all segments",
	Long:    "check that disk space usage is less than 80% on all segments",
	Aliases: []string{"du"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return commanders.NewDiskSpaceCheckerCmd()
	},
}
var subCheckObjectCount = &cobra.Command{
	Use:     "object-count",
	Short:   "count database objects and numeric objects",
	Long:    "count database objects and numeric objects",
	Aliases: []string{"oc"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return commanders.NewObjectCountCheckerCmd()
	},
}
var subCheckSeginstall = &cobra.Command{
	Use:   "seginstall",
	Short: "confirms that the new software is installed on all segments",
	Long: "Running this command will validate that the new software is installed on all segments, " +
		"and register successful or failed validation (available in `gpupgrade status upgrade`)",
	Run: func(cmd *cobra.Command, args []string) {
		err := commanders.NewSeginstallCheckerCmd()
		if err != nil {
			gplog.Error(err.Error())
			os.Exit(1)
		}

		fmt.Println("Seginstall is underway. Use command \"gpupgrade status upgrade\" " +
			"to check its current status, and/or hub logs for possible errors.")
	},
}
var subCheckVersion = &cobra.Command{
	Use:     "version",
	Short:   "validate current version is upgradable",
	Long:    `validate current version is upgradable`,
	Aliases: []string{"ver"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return commanders.NewVersionCheckerCmd()
	},
}

//////////////////////////////////////// CONFIG and its subcommands
var config = &cobra.Command{
	Use:   "config",
	Short: "subcommands to set parameters for subsequent gpupgrade commands",
	Long:  "subcommands to set parameters for subsequent gpupgrade commands",
}

/* NOTE: since we pass a map to the actual implementation, the order of setting
   multiple keys is random */
func createConfigSetSubcommand() *cobra.Command {
	subSet := &cobra.Command{
		Use:   "set",
		Short: "set an upgrade parameter",
		Long:  "set an upgrade parameter",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().NFlag() == 0 {
				return errors.New("the set command requires at least one flag to be specified")
			}
			flagMap := make(map[string]string)
			cmd.Flags().Visit(func(flag *pflag.Flag) { flagMap[flag.Name] = flag.Value.String() })

			return commanders.NewConfigSetterCmd(flagMap) //REVISIT: how to specify a "const flagMap" in golang?

		},
	}

	subSet.Flags().String("old-bindir", "", "install directory for old gpdb version")
	subSet.Flags().String("new-bindir", "", "install directory for new gpdb version")

	return subSet
}

/* NOTE: for simplicity in testing, we show the values in the order they are passed in by the caller */
func createConfigShowSubcommand() *cobra.Command {
	subShow := &cobra.Command{
		Use:   "show",
		Short: "show configuration settings",
		Long:  "show configuration settings",
		RunE: func(cmd *cobra.Command, args []string) error {

			/* place all show flag values in a slice; use a slice to preserve the order of calling */
			var flagList []string

			getRequest := func(flag *pflag.Flag) {
				if flag.Name != "help" {
					flagList = append(flagList, flag.Name)
				}
			}

			if cmd.Flags().NFlag() > 0 {
				cmd.Flags().Visit(getRequest)
			} else {
				cmd.Flags().VisitAll(getRequest) //none specified means show all config values
			}

			configMap, err := commanders.NewConfigShowerCmd(flagList)
			if err != nil {
				return err
			}
			for _, k := range flagList {
				if _, ok := configMap[k]; !ok {
					return errors.New("(internal error): value " + k + " not found")
				}
			}
			if len(flagList) == 1 {
				// Don't prefix with the setting name if the user only asked for one.
				fmt.Println(configMap[flagList[0]])
			} else {
				for _, k := range flagList {
					fmt.Printf("%s - %s\n", k, configMap[k])
				}
			}

			return err
		},
	}

	subShow.Flags().Bool("old-bindir", false, "show install directory for old gpdb version")
	subShow.Flags().Bool("new-bindir", false, "show install directory for new gpdb version")

	return subShow
}

//////////////////////////////////////// PREPARE and its subcommands
var prepare = &cobra.Command{
	Use:   "prepare",
	Short: "subcommands to help you get ready for a gpupgrade",
	Long:  "subcommands to help you get ready for a gpupgrade",
}

func createPrepareInitSubcommand() *cobra.Command {
	var oldBinDir, newBinDir string

	subInit := &cobra.Command{
		Use:   "init",
		Short: "Setup state dir and config file",
		Long:  `Setup state dir and config file`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// If we got here, the args are okay and the user doesn't need a usage
			// dump on failure.
			cmd.SilenceUsage = true
			return commanders.DoInitCmd(oldBinDir, newBinDir)
		},
	}

	subInit.PersistentFlags().StringVar(&oldBinDir, "old-bindir", "", "install directory for old gpdb version")
	subInit.MarkPersistentFlagRequired("old-bindir")
	subInit.PersistentFlags().StringVar(&newBinDir, "new-bindir", "", "install directory for new gpdb version")
	subInit.MarkPersistentFlagRequired("new-bindir")

	return subInit
}

var subPrepareInitCluster = &cobra.Command{
	Use:   "init-cluster",
	Short: "inits the cluster",
	Long:  "Current assumptions is that the cluster already exists. And will only generate json config file for now.",
	Run: func(cmd *cobra.Command, args []string) {

		err := commanders.NewPreparerInitCmd()
		if err != nil {
			gplog.Error(err.Error())
			os.Exit(1)
		}
	},
}
var subPrepareShutdownClusters = &cobra.Command{
	Use:   "shutdown-clusters",
	Short: "shuts down both old and new cluster",
	Long:  "Current assumptions is both clusters exist.",
	Run: func(cmd *cobra.Command, args []string) {
		err := commanders.NewPreparerShutdownCmd()
		if err != nil {
			gplog.Error(err.Error())
			os.Exit(1)
		}
	},
}
var subPrepareStartAgents = &cobra.Command{
	Use:   "start-agents",
	Short: "start agents on segment hosts",
	Long:  "start agents on all segments",
	Run: func(cmd *cobra.Command, args []string) {
		err := commanders.StartAgentsCmd()
		if err != nil {
			gplog.Error(err.Error())
			os.Exit(1)
		}
	},
}
var subPrepareStartHub = &cobra.Command{
	Use:   "start-hub",
	Short: "starts the hub",
	Long:  "starts the hub",
	Run: func(cmd *cobra.Command, args []string) {

		err := commanders.StartHubCmd()
		if err != nil {
			gplog.Error(err.Error())
			os.Exit(1)
		}

		err = commanders.VerifyConnectivityCmd()
		if err != nil {
			gplog.Error("gpupgrade is unable to connect via gRPC to the hub")
			gplog.Error("%v", err)
			os.Exit(1)
		}
	},
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
		strOutput, err := commanders.OverallConversionStatusCmd()
		if err != nil {
			gplog.Error(err.Error())
			os.Exit(1)
		}
		fmt.Print(strOutput)
	},
}
var subStatusUpgrade = &cobra.Command{
	Use:   "upgrade",
	Short: "the status of the upgrade",
	Long:  "the status of the upgrade",
	Run: func(cmd *cobra.Command, args []string) {
		strOutput, err := commanders.OverallUpgradeStatusCmd()
		if err != nil {
			gplog.Error(err.Error())
			os.Exit(1)
		}
		fmt.Print(strOutput)
	},
}

//////////////////////////////////////// UPGRADE and its subcommands
var upgrade = &cobra.Command{
	Use:   "upgrade",
	Short: "starts upgrade process",
	Long:  `starts upgrade process`,
}

var subUpgradeConvertMaster = &cobra.Command{
	Use:   "convert-master",
	Short: "start upgrade process on master",
	Long:  `start upgrade process on master`,
	Run: func(cmd *cobra.Command, args []string) {
		err := commanders.ConvertMasterCmd()
		if err != nil {
			gplog.Error(err.Error())
			os.Exit(1)
		}
	},
}
var subUpgradeConvertPrimaries = &cobra.Command{
	Use:   "convert-primaries",
	Short: "start upgrade process on primary segments",
	Long:  `start upgrade process on primary segments`,
	Run: func(cmd *cobra.Command, args []string) {
		err := commanders.ConvertPrimariesCmd()
		if err != nil {
			gplog.Error(err.Error())
			os.Exit(1)
		}
	},
}
var subUpgradeReconfigurePorts = &cobra.Command{
	Use:   "reconfigure-ports",
	Short: "Set master port on upgraded cluster to the value from the older cluster",
	Long:  `Set master port on upgraded cluster to the value from the older cluster`,
	Run: func(cmd *cobra.Command, args []string) {
		err := commanders.ReconfigurePortsCmd()
		if err != nil {
			gplog.Error(err.Error())
			os.Exit(1)
		}
	},
}
var subUpgradeShareOids = &cobra.Command{
	Use:   "share-oids",
	Short: "share oid files across cluster",
	Long:  `share oid files generated by pg_upgrade on master, across cluster`,
	Run: func(cmd *cobra.Command, args []string) {
		err := commanders.ShareOidsCmd()
		if err != nil {
			gplog.Error(err.Error())
			os.Exit(1)
		}
	},
}
var subUpgradeValidateStartCluster = &cobra.Command{
	Use:   "validate-start-cluster",
	Short: "Attempt to start upgraded cluster",
	Long:  `Use gpstart in order to validate that the new cluster can successfully transition from a stopped to running state`,
	Run: func(cmd *cobra.Command, args []string) {
		err := commanders.ValidateStartClusterCmd()
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
		fmt.Println(commanders.VersionString())
	},
}
