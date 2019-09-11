package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/pkg/errors"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/daemon"
	"github.com/greenplum-db/gpupgrade/utils/log"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

// This directory to have the implementation code for the gRPC server to serve
// Minimal CLI command parsing to embrace that booting this binary to run the hub might have some flags like a log dir

func main() {
	var logdir string
	var shouldDaemonize bool
	var doLogVersionAndExit bool

	var RootCmd = &cobra.Command{
		Use:   os.Args[0],
		Short: "Start the gpupgrade_hub (blocks)",
		Long:  `Start the gpupgrade_hub (blocks)`,
		Args:  cobra.MaximumNArgs(0), //no positional args allowed
		RunE: func(cmd *cobra.Command, args []string) error {
			gplog.InitializeLogging("gpupgrade_hub", logdir)
			debug.SetTraceback("all")
			defer log.WritePanics()

			if doLogVersionAndExit {
				fmt.Println(utils.VersionString("gpupgrade_hub"))
				gplog.Info(utils.VersionString("gpupgrade_hub"))
				os.Exit(0)
			}

			conf := &services.HubConfig{
				CliToHubPort:   7527,
				HubToAgentPort: 6416,
				StateDir:       utils.GetStateDir(),
				LogDir:         logdir,
			}

			finfo, err := os.Stat(conf.StateDir)
			if os.IsNotExist(err) {
				return fmt.Errorf("gpupgrade state dir (%s) does not exist. Did you run gpupgrade initialize?", conf.StateDir)
			} else if err != nil {
				return err
			} else if !finfo.IsDir() {
				return fmt.Errorf("gpupgrade state dir (%s) does not exist as a directory.", conf.StateDir)
			}

			// the hub needs to be able to be restarted at any time, including
			//  the first time.  So we populate the cluster here.
			// TODO: design a better scheme for this.
			source := &utils.Cluster{
				ConfigPath: filepath.Join(conf.StateDir, utils.SOURCE_CONFIG_FILENAME),
			}
			target := &utils.Cluster{
				ConfigPath: filepath.Join(conf.StateDir, utils.TARGET_CONFIG_FILENAME),
			}

			errSource := source.Load()
			errTarget := target.Load()
			if errSource != nil && errTarget != nil {
				errBoth := errors.Errorf("Source error: %s\nTarget error: %s", errSource.Error(), errTarget.Error())
				return errors.Wrap(errBoth, "Unable to load source or target cluster configuration")
			} else if errSource != nil {
				return errors.Wrap(errSource, "Unable to load source cluster configuration")
			} else if errTarget != nil {
				return errors.Wrap(errTarget, "Unable to load target cluster configuration")
			}

			cm := upgradestatus.NewChecklistManager(conf.StateDir)

			hub := services.NewHub(source, target, grpc.DialContext, conf, cm)

			// Set up the checklist steps in order.
			//
			// TODO: make sure the implementations here, and the Checklist below, are
			// fully exercised in end-to-end tests. It feels like we should be able to
			// pull these into a Hub method or helper function, but currently the
			// interfaces aren't well componentized.
			cm.AddWritableStep(upgradestatus.CONFIG, idl.UpgradeSteps_CONFIG)
			cm.AddWritableStep(upgradestatus.START_AGENTS, idl.UpgradeSteps_START_AGENTS)
			cm.AddWritableStep(upgradestatus.INIT_CLUSTER, idl.UpgradeSteps_INIT_CLUSTER)
			cm.AddWritableStep(upgradestatus.SHUTDOWN_CLUSTERS, idl.UpgradeSteps_SHUTDOWN_CLUSTERS)
			cm.AddWritableStep(upgradestatus.CONVERT_MASTER, idl.UpgradeSteps_CONVERT_MASTER)
			cm.AddWritableStep(upgradestatus.COPY_MASTER, idl.UpgradeSteps_COPY_MASTER)

			cm.AddReadOnlyStep(upgradestatus.CONVERT_PRIMARIES, idl.UpgradeSteps_CONVERT_PRIMARIES,
				func(stepName string) idl.StepStatus {
					return services.PrimaryConversionStatus(hub)
				})

			cm.AddWritableStep(upgradestatus.VALIDATE_START_CLUSTER, idl.UpgradeSteps_VALIDATE_START_CLUSTER)
			cm.AddWritableStep(upgradestatus.RECONFIGURE_PORTS, idl.UpgradeSteps_RECONFIGURE_PORTS)

			if shouldDaemonize {
				hub.MakeDaemon()
			}

			err = hub.Start()
			if err != nil {
				return err
			}

			return nil
		},
	}

	RootCmd.PersistentFlags().StringVar(&logdir, "log-directory", "", "gpupgrade_hub log directory")

	daemon.MakeDaemonizable(RootCmd, &shouldDaemonize)
	utils.VersionAddCmdlineOption(RootCmd, &doLogVersionAndExit)

	err := RootCmd.Execute()
	if err != nil && err != daemon.ErrSuccessfullyDaemonized {
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
