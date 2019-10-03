package commanders

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/idl"
)

var lines = map[idl.UpgradeSteps]string{
	idl.UpgradeSteps_INIT_CLUSTER:           "Creating new cluster...",
	idl.UpgradeSteps_SHUTDOWN_CLUSTERS:      "Stopping clusters...",
	idl.UpgradeSteps_CONVERT_MASTER:         "Upgrading master...",
	idl.UpgradeSteps_COPY_MASTER:            "Copying master to segments...",
	idl.UpgradeSteps_CONVERT_PRIMARIES:      "Upgrading segments...",
	idl.UpgradeSteps_VALIDATE_START_CLUSTER: "Starting upgraded cluster...",
}

var indicators = map[idl.StepStatus]string{
	idl.StepStatus_RUNNING:  "[IN PROGRESS]",
	idl.StepStatus_COMPLETE: "[COMPLETE]",
	idl.StepStatus_FAILED:   "[FAILED]",
}

func Execute(client idl.CliToHubClient, verbose bool) error {
	fmt.Println("\nExecute in progress.")
	if verbose {
		fmt.Println()
	}

	stream, err := client.Execute(context.Background(), &idl.ExecuteRequest{})
	if err != nil {
		// TODO: Change the logging message?
		gplog.Error("ERROR - Unable to connect to hub")
		return err
	}

	var lastStep idl.UpgradeSteps
	for {
		var msg *idl.UpgradeMessage
		msg, err = stream.Recv()
		if err != nil {
			break
		}

		switch x := msg.Contents.(type) {
		case *idl.UpgradeMessage_Chunk:
			if !verbose {
				continue
			}

			if x.Chunk.Type == idl.Chunk_STDOUT {
				os.Stdout.Write(x.Chunk.Buffer)
			} else if x.Chunk.Type == idl.Chunk_STDERR {
				os.Stderr.Write(x.Chunk.Buffer)
			}

		case *idl.UpgradeMessage_Status:
			line, ok := lines[x.Status.Step]
			if !ok {
				panic(fmt.Sprintf("unexpected step %#v", x.Status.Step))
			}

			indicator, ok := indicators[x.Status.Status]
			if !ok {
				panic(fmt.Sprintf("unexpected status %#v", x.Status.Status))
			}

			// Rewrite the current line whenever we get an update for the
			// current step. (This behavior is switched off in verbose mode,
			// because it interferes with the output stream.)
			if !verbose {
				if x.Status.Step == lastStep {
					fmt.Print("\r")
				} else {
					fmt.Println()
				}
			}
			lastStep = x.Status.Step

			fmt.Printf("%-67s%-13s", line, indicator)
			if verbose {
				fmt.Println()
			}

		default:
			panic(fmt.Sprintf("Unknown message type for Execute: %T", x))
		}
	}

	if !verbose {
		fmt.Println()
	}

	if err != io.EOF {
		return err
	}

	fmt.Println(`
You may now run queries against the new database and perform any other
validation desired prior to finalizing your upgrade.

WARNING: If any queries modify the database during this time, this will affect
your revert time.

If you are satisfied with the state of the cluster, run "gpupgrade finalize" on
the command line to finish the upgrade.

If you would like to return the cluster to its original state, run
"gpupgrade revert" on the command line.`)

	return nil
}
