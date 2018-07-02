package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/agent/services"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/spf13/cobra"
)

func main() {
	//debug.SetTraceback("all")
	//parser := flags.NewParser(&AllServices, flags.HelpFlag|flags.PrintErrors)
	//
	//_, err := parser.Parse()
	//if err != nil {
	//	os.Exit(utils.GetExitCodeForError(err))
	//}
	var logdir, statedir string
	var daemonize bool
	var daemon bool
	var RootCmd = &cobra.Command{
		Use:   "gpupgrade_agent ",
		Short: "Start the Command Listener (blocks)",
		Long:  `Start the Command Listener (blocks)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			gplog.InitializeLogging("gpupgrade_agent", logdir)

			if daemon && terminal.IsTerminal(int(os.Stdout.Fd())) {
				// Shouldn't be calling this from the command line.
				return fmt.Errorf("--daemon is an internal option (did you mean --daemonize?)")
			}

			// TODO: this is all copy-pasted from the hub code. Consolidate!
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

			conf := services.AgentConfig{
				Port:     6416,
				StateDir: statedir,
			}

			agentServer := services.NewAgentServer(&cluster.GPDBExecutor{}, conf)
			if daemon {
				agentServer.MakeDaemon()
			}

			agentServer.Start()

			agentServer.Stop()

			return nil
		},
	}

	RootCmd.Flags().StringVar(&logdir, "log-directory", "", "command_listener log directory")
	RootCmd.Flags().StringVar(&statedir, "state-directory", utils.GetStateDir(), "Agent state directory")

	RootCmd.Flags().BoolVar(&daemonize, "daemonize", false, "start hub in the background")
	RootCmd.Flags().BoolVar(&daemon, "daemon", false, "disconnect standard streams (internal option; use --daemonize instead)")
	RootCmd.Flags().MarkHidden("daemon")

	if err := RootCmd.Execute(); err != nil {
		if gplog.GetLogger() == nil {
			// In case we didn't get through RootCmd.Execute(), set up logging
			// here. Otherwise we crash.
			// XXX it'd be really nice to have a "ReinitializeLogging" building
			// block somewhere.
			gplog.InitializeLogging("gpupgrade_agent", "")
		}

		gplog.Error(err.Error())
		os.Exit(1)
	}
}
