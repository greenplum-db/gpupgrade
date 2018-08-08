package main

import (
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/daemon"
	"github.com/greenplum-db/gpupgrade/utils/log"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

// This directory to have the implementation code for the gRPC server to serve
// Minimal CLI command parsing to embrace that booting this binary to run the hub might have some flags like a log dir

func main() {
	var logdir string
	var shouldDaemonize bool

	var RootCmd = &cobra.Command{
		Use:   os.Args[0],
		Short: "Start the gpupgrade_hub (blocks)",
		Long:  `Start the gpupgrade_hub (blocks)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			gplog.InitializeLogging("gpupgrade_hub", logdir)
			debug.SetTraceback("all")
			defer log.WritePanics()

			conf := &services.HubConfig{
				CliToHubPort:   7527,
				HubToAgentPort: 6416,
				StateDir:       utils.GetStateDir(),
				LogDir:         logdir,
			}
			source := &utils.Cluster{ConfigPath: filepath.Join(conf.StateDir, utils.SOURCE_CONFIG_FILENAME)}
			target := &utils.Cluster{ConfigPath: filepath.Join(conf.StateDir, utils.TARGET_CONFIG_FILENAME)}
			cm := upgradestatus.NewChecklistManager(conf.StateDir)

			// Load the cluster configuration.
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

			hub := services.NewHub(source, target, grpc.DialContext, conf, cm)

			// Set up the checklist steps in order.
			//
			// TODO: make sure the implementations here, and the Checklist below, are
			// fully exercised in end-to-end tests. It feels like we should be able to
			// pull these into a Hub method or helper function, but currently the
			// interfaces aren't well componentized.
			cm.AddWritableStep(upgradestatus.CONFIG, pb.UpgradeSteps_CONFIG)
			cm.AddWritableStep(upgradestatus.SEGINSTALL, pb.UpgradeSteps_SEGINSTALL)
			cm.AddWritableStep(upgradestatus.INIT_CLUSTER, pb.UpgradeSteps_INIT_CLUSTER)

			cm.AddWritableStep(upgradestatus.SHUTDOWN_CLUSTERS, pb.UpgradeSteps_SHUTDOWN_CLUSTERS)

			cm.AddReadOnlyStep(upgradestatus.CONVERT_MASTER, pb.UpgradeSteps_CONVERT_MASTER,
				func(stepName string) pb.StepStatus {
					convertMasterPath := filepath.Join(conf.StateDir, stepName)
					sourceDataDir := source.MasterDataDir()
					return upgradestatus.SegmentConversionStatus(convertMasterPath, sourceDataDir, source.Executor)
				})

			cm.AddWritableStep(upgradestatus.START_AGENTS, pb.UpgradeSteps_START_AGENTS)
			cm.AddWritableStep(upgradestatus.SHARE_OIDS, pb.UpgradeSteps_SHARE_OIDS)

			cm.AddReadOnlyStep(upgradestatus.CONVERT_PRIMARIES, pb.UpgradeSteps_CONVERT_PRIMARIES,
				func(stepName string) pb.StepStatus {
					return services.PrimaryConversionStatus(hub)
				})

			cm.AddWritableStep(upgradestatus.VALIDATE_START_CLUSTER, pb.UpgradeSteps_VALIDATE_START_CLUSTER)
			cm.AddWritableStep(upgradestatus.RECONFIGURE_PORTS, pb.UpgradeSteps_RECONFIGURE_PORTS)

			if shouldDaemonize {
				hub.MakeDaemon()
			}

			err := hub.Start()
			if err != nil {
				return err
			}

			hub.Stop()

			return nil
		},
	}

	RootCmd.PersistentFlags().StringVar(&logdir, "log-directory", "", "gpupgrade_hub log directory")

	daemon.MakeDaemonizable(RootCmd, &shouldDaemonize)

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
