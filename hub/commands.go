package hub

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/pkg/errors"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/daemon"
	"github.com/greenplum-db/gpupgrade/utils/log"
)

// This directory to have the implementation code for the gRPC server to serve
// Minimal CLI command parsing to embrace that booting this binary to run the hub might have some flags like a log dir

func Command() *cobra.Command {
	var logdir string
	var shouldDaemonize bool

	var cmd = &cobra.Command{
		Use:    "hub",
		Short:  "Start the gpupgrade hub (blocks)",
		Long:   `Start the gpupgrade hub (blocks)`,
		Hidden: true,
		Args:   cobra.MaximumNArgs(0), //no positional args allowed
		RunE: func(cmd *cobra.Command, args []string) error {
			gplog.InitializeLogging("gpupgrade hub", logdir)
			debug.SetTraceback("all")
			defer log.WritePanics()

			conf := &Config{
				CliToHubPort:   7527,
				HubToAgentPort: 6416,
				LogDir:         logdir,
			}

			finfo, err := os.Stat(utils.GetStateDir())
			if os.IsNotExist(err) {
				return fmt.Errorf("gpupgrade state dir (%s) does not exist. Did you run gpupgrade initialize?", utils.GetStateDir())
			} else if err != nil {
				return err
			} else if !finfo.IsDir() {
				return fmt.Errorf("gpupgrade state dir (%s) does not exist as a directory.", utils.GetStateDir())
			}

			// the hub needs to be able to be restarted at any time, including
			//  the first time.  So we populate the cluster here.
			// TODO: design a better scheme for this.
			source := &utils.Cluster{
				ConfigPath: filepath.Join(utils.GetStateDir(), utils.SOURCE_CONFIG_FILENAME),
			}
			target := &utils.Cluster{
				ConfigPath: filepath.Join(utils.GetStateDir(), utils.TARGET_CONFIG_FILENAME),
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

			cm := upgradestatus.NewChecklistManager(utils.GetStateDir())

			h := New(source, target, grpc.DialContext, conf, cm)

			// Set up the checklist steps in order.
			//
			// TODO: make sure the implementations here, and the Checklist below, are
			// fully exercised in end-to-end tests. It feels like we should be able to
			// pull these into a Hub method or helper function, but currently the
			// interfaces aren't well componentized.
			cm.AddWritableStep(upgradestatus.CONFIG, idl.UpgradeSteps_CONFIG)
			cm.AddWritableStep(upgradestatus.START_AGENTS, idl.UpgradeSteps_START_AGENTS)
			cm.AddWritableStep(upgradestatus.CREATE_TARGET_CONFIG, idl.UpgradeSteps_CREATE_TARGET_CONFIG)
			cm.AddWritableStep(upgradestatus.SHUTDOWN_SOURCE_CLUSTER, idl.UpgradeSteps_SHUTDOWN_SOURCE_CLUSTER)
			cm.AddWritableStep(upgradestatus.INIT_TARGET_CLUSTER, idl.UpgradeSteps_INIT_TARGET_CLUSTER)
			cm.AddWritableStep(upgradestatus.SHUTDOWN_TARGET_CLUSTER, idl.UpgradeSteps_SHUTDOWN_TARGET_CLUSTER)
			cm.AddWritableStep(upgradestatus.CHECK_UPGRADE, idl.UpgradeSteps_CHECK_UPGRADE)
			cm.AddWritableStep(upgradestatus.UPGRADE_MASTER, idl.UpgradeSteps_UPGRADE_MASTER)
			cm.AddWritableStep(upgradestatus.COPY_MASTER, idl.UpgradeSteps_COPY_MASTER)
			cm.AddWritableStep(upgradestatus.UPGRADE_PRIMARIES, idl.UpgradeSteps_UPGRADE_PRIMARIES)
			cm.AddWritableStep(upgradestatus.START_TARGET_CLUSTER, idl.UpgradeSteps_START_TARGET_CLUSTER)
			cm.AddWritableStep(upgradestatus.RECONFIGURE_PORTS, idl.UpgradeSteps_RECONFIGURE_PORTS)

			if shouldDaemonize {
				h.MakeDaemon()
			}

			err = h.Start()
			if err != nil {
				return err
			}

			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&logdir, "log-directory", "", "gpupgrade hub log directory")

	daemon.MakeDaemonizable(cmd, &shouldDaemonize)

	return cmd
}
