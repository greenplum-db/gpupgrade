package main

import (
	"fmt"
	"os"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/agent/services"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/daemon"
	"github.com/greenplum-db/gpupgrade/utils/log"
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
	var shouldDaemonize bool
	var doLogVersionAndExit bool

	var RootCmd = &cobra.Command{
		Use:   "gpupgrade_agent ",
		Short: "Start the Command Listener (blocks)",
		Long:  `Start the Command Listener (blocks)`,
		Args:  cobra.MaximumNArgs(0), //no positional args allowed
		RunE: func(cmd *cobra.Command, args []string) error {
			gplog.InitializeLogging("gpupgrade_agent", logdir)
			defer log.WritePanics()

			if doLogVersionAndExit {
				fmt.Println(utils.VersionString("gpupgrade_agent"))
				gplog.Info(utils.VersionString("gpupgrade_agent"))
				os.Exit(0)
			}

			conf := services.AgentConfig{
				Port:     6416,
				StateDir: statedir,
			}

			agentServer := services.NewAgentServer(&cluster.GPDBExecutor{}, conf)
			if shouldDaemonize {
				agentServer.MakeDaemon()
			}

			agentServer.Start()

			agentServer.Stop()

			return nil
		},
	}

	RootCmd.Flags().StringVar(&logdir, "log-directory", "", "command_listener log directory")
	RootCmd.Flags().StringVar(&statedir, "state-directory", utils.GetStateDir(), "Agent state directory")

	daemon.MakeDaemonizable(RootCmd, &shouldDaemonize)
	utils.VersionAddCmdlineOption(RootCmd, &doLogVersionAndExit)

	err := RootCmd.Execute()
	if err != nil && err != daemon.ErrSuccessfullyDaemonized {
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
