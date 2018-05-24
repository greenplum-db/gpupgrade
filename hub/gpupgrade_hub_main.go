package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"github.com/greenplum-db/gpupgrade/helpers"
	"github.com/greenplum-db/gpupgrade/hub/cluster"
	"github.com/greenplum-db/gpupgrade/hub/configutils"
	"github.com/greenplum-db/gpupgrade/hub/services"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"github.com/greenplum-db/gpupgrade/hub/cluster_ssher"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"time"
	"runtime/debug"
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
				StateDir:       filepath.Join(os.Getenv("HOME"), ".gpupgrade"),
				LogDir:         logdir,
			}
			reader := configutils.NewReader()

			commandExecer := func(command string, vars ...string) helpers.Command {
				return exec.Command(command, vars...)
			}

			clusterPair := cluster.NewClusterPair(conf.StateDir, commandExecer)

			clusterSsher := cluster_ssher.NewClusterSsher(
				upgradestatus.NewChecklistManager(conf.StateDir),
				services.NewPingerManager(conf.StateDir, 500*time.Millisecond),
				commandExecer,
			)
			hub := services.NewHub(clusterPair, &reader, grpc.DialContext, commandExecer, conf, clusterSsher)
			hub.Start()

			hub.Stop()

			return nil
		},
	}

	RootCmd.PersistentFlags().StringVar(&logdir, "log-directory", "", "gpupgrade_hub log directory")

	if err := RootCmd.Execute(); err != nil {
		gplog.Error(err.Error())
		os.Exit(1)
	}
}
