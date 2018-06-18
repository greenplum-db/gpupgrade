package main

import (
	"os"
	"os/exec"
	"runtime/debug"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/helpers"
	"github.com/greenplum-db/gpupgrade/hub/cluster_ssher"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

// This directory to have the implementation code for the gRPC server to serve
// Minimal CLI command parsing to embrace that booting this binary to run the hub might have some flags like a log dir

func main() {
	var logdir string
	var RootCmd = &cobra.Command{
		Use:   "gpupgrade_hub [--log-directory path]",
		Short: "Start the gpupgrade_hub (blocks)",
		Long:  `Start the gpupgrade_hub (blocks)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			gplog.InitializeLogging("gpupgrade_hub", logdir)
			debug.SetTraceback("all")

			conf := &services.HubConfig{
				CliToHubPort:   7527,
				HubToAgentPort: 6416,
				StateDir:       utils.GetStateDir(),
				LogDir:         logdir,
			}
			commandExecer := func(command string, vars ...string) helpers.Command {
				return exec.Command(command, vars...)
			}
			cm := upgradestatus.NewChecklistManager(conf.StateDir)
			clusterSsher := cluster_ssher.NewClusterSsher(
				cm,
				services.NewPingerManager(conf.StateDir, 500*time.Millisecond),
				commandExecer,
			)
			hub := services.NewHub(&services.ClusterPair{}, grpc.DialContext, commandExecer, conf, clusterSsher, cm)
			hub.Start()

			hub.Stop()

			return nil
		},
	}

	RootCmd.PersistentFlags().StringVar(&logdir, "log-directory", "", "gpupgrade_hub log directory")

	if err := RootCmd.Execute(); err != nil {
		if gplog.GetLogger() == nil {
			// In case we didn't get through RootCmd.Execute(), set up logging
			// here. Otherwise we crash.
			// XXX it'd be really nice to have a "ReinitializeLogging" building
			// block somewhere.
			gplog.InitializeLogging("gpupgrade_hub", "")
		}

		gplog.Error(err.Error())
		os.Exit(1)
	}
}
