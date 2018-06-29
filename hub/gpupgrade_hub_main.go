package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/helpers"
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
	var daemonize bool
	var daemon bool
	var RootCmd = &cobra.Command{
		Use:   os.Args[0],
		Short: "Start the gpupgrade_hub (blocks)",
		Long:  `Start the gpupgrade_hub (blocks)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			gplog.InitializeLogging("gpupgrade_hub", logdir)
			debug.SetTraceback("all")

			if daemon && terminal.IsTerminal(int(os.Stdout.Fd())) {
				// Shouldn't be calling this from the command line.
				return fmt.Errorf("--daemon is an internal option (did you mean --daemonize?)")
			}

			if daemonize {
				// Strip out the --daemonize option and add --daemon.
				daemonArgs := make([]string, 0)
				for _, arg := range os.Args[1:] {
					if arg == "--daemonize" {
						arg = "--daemon"
					}
					daemonArgs = append(daemonArgs, arg)
				}

				command := exec.Command(os.Args[0], daemonArgs...)
				// TODO: what's a good timeout?
				err := utils.Daemonize(command, os.Stdout, os.Stderr, 2*time.Second)

				if err != nil {
					exitError, ok := err.(*exec.ExitError)
					if ok {
						// Exit with the same code as the child, if we can
						// figure it out.
						code := 1

						status, ok := exitError.Sys().(syscall.WaitStatus)
						if ok {
							code = status.ExitStatus()
						}

						os.Exit(code)
					}

					// Otherwise, deal with the error normally.
				}

				return err
			}

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
			hub := services.NewHub(&utils.ClusterPair{}, grpc.DialContext, commandExecer, conf, cm)
			if daemon {
				hub.MakeDaemon()
			}

			hub.Start()

			hub.Stop()

			return nil
		},
	}

	RootCmd.PersistentFlags().StringVar(&logdir, "log-directory", "", "gpupgrade_hub log directory")

	RootCmd.Flags().BoolVar(&daemonize, "daemonize", false, "start hub in the background")
	RootCmd.Flags().BoolVar(&daemon, "daemon", false, "disconnect standard streams (internal option; use --daemonize instead)")
	RootCmd.Flags().MarkHidden("daemon")

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
