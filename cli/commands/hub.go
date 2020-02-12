package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/daemon"
	"github.com/greenplum-db/gpupgrade/utils/log"
)

// This directory to have the implementation code for the gRPC server to serve
// Minimal CLI command parsing to embrace that booting this binary to run the hub might have some flags like a log dir

func Hub() *cobra.Command {
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

			stateDir := utils.GetStateDir()
			finfo, err := os.Stat(stateDir)
			if os.IsNotExist(err) {
				return fmt.Errorf("gpupgrade state dir (%s) does not exist. Did you run gpupgrade initialize?", stateDir)
			} else if err != nil {
				return err
			} else if !finfo.IsDir() {
				return fmt.Errorf("gpupgrade state dir (%s) does not exist as a directory.", stateDir)
			}

			// Load the hub persistent configuration.
			//
			// they're not defined in the configuration (as happens
			// pre-initialize), we still need good defaults.
			conf := &hub.Config{
				Port:        7527,
				AgentPort:   6416,
				UseLinkMode: false,
			}

			path := filepath.Join(stateDir, hub.ConfigFileName)
			err = hub.LoadConfig(conf, path)
			if err != nil {
				return err
			}

			h := hub.New(conf, grpc.DialContext, stateDir)

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
