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
			err := source.Load()
			if err != nil {
				return errors.Wrap(err, "Unable to load source cluster configuration")
			}
			err = target.Load()
			if err != nil {
				return errors.Wrap(err, "Unable to load target cluster configuration")
			}

			hub := services.NewHub(source, target, grpc.DialContext, conf, cm)

			// TODO: make sure the implementations here, and the Checklist below, are
			// fully exercised in end-to-end tests. It feels like we should be able to
			// pull these into a Hub method or helper function, but currently the
			// interfaces aren't well componentized.
			stateCheck := func(step upgradestatus.StateReader) pb.StepStatus {
				checker := upgradestatus.StateCheck{
					Path: filepath.Join(conf.StateDir, step.Name()),
					Step: step.Code(),
				}
				return checker.GetStatus()
			}

			shutDownStatus := func(step upgradestatus.StateReader) pb.StepStatus {
				stepdir := filepath.Join(conf.StateDir, step.Name())
				return upgradestatus.ClusterShutdownStatus(stepdir, source.Executor)
			}

			convertMasterStatus := func(step upgradestatus.StateReader) pb.StepStatus {
				convertMasterPath := filepath.Join(conf.StateDir, step.Name())
				sourceDataDir := source.MasterDataDir()
				return upgradestatus.SegmentConversionStatus(convertMasterPath, sourceDataDir, source.Executor)
			}

			convertPrimariesStatus := func(step upgradestatus.StateReader) pb.StepStatus {
				return services.PrimaryConversionStatus(hub)
			}

			cm.LoadSteps([]upgradestatus.Step{
				{upgradestatus.CONFIG, pb.UpgradeSteps_CONFIG, stateCheck},
				{upgradestatus.SEGINSTALL, pb.UpgradeSteps_SEGINSTALL, stateCheck},
				{upgradestatus.INIT_CLUSTER, pb.UpgradeSteps_INIT_CLUSTER, stateCheck},
				{upgradestatus.SHUTDOWN_CLUSTERS, pb.UpgradeSteps_SHUTDOWN_CLUSTERS, shutDownStatus},
				{upgradestatus.CONVERT_MASTER, pb.UpgradeSteps_CONVERT_MASTER, convertMasterStatus},
				{upgradestatus.START_AGENTS, pb.UpgradeSteps_START_AGENTS, stateCheck},
				{upgradestatus.SHARE_OIDS, pb.UpgradeSteps_SHARE_OIDS, stateCheck},
				{upgradestatus.CONVERT_PRIMARIES, pb.UpgradeSteps_CONVERT_PRIMARIES, convertPrimariesStatus},
				{upgradestatus.VALIDATE_START_CLUSTER, pb.UpgradeSteps_VALIDATE_START_CLUSTER, stateCheck},
				{upgradestatus.RECONFIGURE_PORTS, pb.UpgradeSteps_RECONFIGURE_PORTS, stateCheck},
			})

			if shouldDaemonize {
				hub.MakeDaemon()
			}

			err = hub.Start()
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
	if err != nil && err != daemon.SuccessfullyDaemonized {
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
