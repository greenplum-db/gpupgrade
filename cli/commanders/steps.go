package commanders

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
)

type receiver interface {
	Recv() (*idl.Message, error)
}

var lines = map[idl.UpgradeSteps]string{
	idl.UpgradeSteps_CONFIG:                  "Retrieving configs...",
	idl.UpgradeSteps_START_AGENTS:            "Starting agents...",
	idl.UpgradeSteps_CREATE_TARGET_CONFIG:    "Generating new cluster configuration...",
	idl.UpgradeSteps_SHUTDOWN_SOURCE_CLUSTER: "Stopping old cluster...",
	idl.UpgradeSteps_INIT_TARGET_CLUSTER:     "Creating new cluster...",
	idl.UpgradeSteps_SHUTDOWN_TARGET_CLUSTER: "Stopping new cluster...",
	idl.UpgradeSteps_CHECK_UPGRADE:           "Running pg_upgrade checks...",
	idl.UpgradeSteps_UPGRADE_MASTER:          "Upgrading master...",
	idl.UpgradeSteps_COPY_MASTER:             "Copying master to segments...",
	idl.UpgradeSteps_UPGRADE_PRIMARIES:       "Upgrading segments...",
	idl.UpgradeSteps_START_TARGET_CLUSTER:    "Starting new cluster...",
	idl.UpgradeSteps_RECONFIGURE_PORTS:       "Changing cluster ports...",
}

var indicators = map[idl.StepStatus]string{
	idl.StepStatus_RUNNING:  "[IN PROGRESS]",
	idl.StepStatus_COMPLETE: "[COMPLETE]",
	idl.StepStatus_FAILED:   "[FAILED]",
}

func Initialize(client idl.CliToHubClient, oldBinDir, newBinDir string, oldPort int, verbose bool) (err error) {

	request := &idl.InitializeRequest{
		OldBinDir: oldBinDir,
		NewBinDir: newBinDir,
		OldPort:   int32(oldPort),
	}

	stream, err := client.Initialize(context.Background(), request)
	if err != nil {
		return errors.Wrap(err, "initializing hub")
	}

	err = UILoop(stream, verbose)
	if err != nil {
		return xerrors.Errorf("Initialize: %w", err)
	}

	return nil
}

func InitializeCreateCluster(client idl.CliToHubClient, verbose bool) (err error) {
	stream, err := client.InitializeCreateCluster(context.Background(), &idl.InitializeCreateClusterRequest{})
	if err != nil {
		return errors.Wrap(err, "initializing hub2")
	}

	err = UILoop(stream, verbose)
	if err != nil {
		return xerrors.Errorf("InitializeCreateCluster: %w", err)
	}

	return nil
}

func Execute(client idl.CliToHubClient, verbose bool) error {
	fmt.Println()
	fmt.Println("Execute in progress.")
	fmt.Println()

	stream, err := client.Execute(context.Background(), &idl.ExecuteRequest{})
	if err != nil {
		// TODO: Change the logging message?
		gplog.Error("ERROR - Unable to connect to hub")
		return err
	}

	err = UILoop(stream, verbose)
	if err != nil {
		return xerrors.Errorf("Execute: %w", err)
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

func Finalize(client idl.CliToHubClient) error {
	fmt.Println()
	fmt.Println("Finalize in progress.")
	fmt.Println()

	stream, err := client.Finalize(context.Background(), &idl.FinalizeRequest{})
	if err != nil {
		gplog.Error(err.Error())
		return err
	}

	err = UILoop(stream, false)
	if err != nil {
		return xerrors.Errorf("Finalize: %w", err)
	}

	// TODO version number
	fmt.Println(`
The cluster is now upgraded and is ready to be used.`)

	return nil
}

func UILoop(stream receiver, verbose bool) error {
	var lastStep idl.UpgradeSteps
	var err error

	for {
		var msg *idl.Message
		msg, err = stream.Recv()
		if err != nil {
			break
		}

		switch x := msg.Contents.(type) {
		case *idl.Message_Chunk:
			if !verbose {
				continue
			}

			if x.Chunk.Type == idl.Chunk_STDOUT {
				os.Stdout.Write(x.Chunk.Buffer)
			} else if x.Chunk.Type == idl.Chunk_STDERR {
				os.Stderr.Write(x.Chunk.Buffer)
			}

		case *idl.Message_Status:
			// Rewrite the current line whenever we get an update for the
			// current step. (This behavior is switched off in verbose mode,
			// because it interferes with the output stream.)
			if !verbose {
				if lastStep == idl.UpgradeSteps_UNKNOWN_STEP {
					// This is the first call, so we don't need to "terminate"
					// the previous line at all.
				} else if x.Status.Step == lastStep {
					fmt.Print("\r")
				} else {
					fmt.Println()
				}
			}
			lastStep = x.Status.Step

			fmt.Printf(FormatStatus(x.Status))
			if verbose {
				fmt.Println()
			}

		default:
			panic(fmt.Sprintf("unknown message type: %T", x))
		}
	}

	if !verbose {
		fmt.Println()
	}

	if err != io.EOF {
		return err
	}

	return nil
}

// FormatStatus returns a status string based on the upgrade status message.
// It's exported for ease of testing.
//
// FormatStatus panics if it doesn't have a string representation for a given
// protobuf code.
func FormatStatus(status *idl.UpgradeStepStatus) string {
	line, ok := lines[status.Step]
	if !ok {
		panic(fmt.Sprintf("unexpected step %#v", status.Step))
	}

	return Format(line, status.Status)
}

// Format is also exported for ease of testing (see FormatStatus). Use Substep
// instead.
func Format(description string, status idl.StepStatus) string {
	indicator, ok := indicators[status]
	if !ok {
		panic(fmt.Sprintf("unexpected status %#v", status))
	}

	return fmt.Sprintf("%-67s%-13s", description, indicator)
}

type substep struct {
	description string
}

// Substep prints out an "in progress" marker for the given substep description,
// and returns a struct that can be .Finish()d (in a defer statement) to print
// the final complete/failed state.
func Substep(description string) *substep {
	fmt.Printf("%s\r", Format(description, idl.StepStatus_RUNNING))
	return &substep{description}
}

// Finish prints out the final status of the substep; either COMPLETE or FAILED
// depending on whether or not there is an error. The method takes a pointer to
// error rather than error to make it possible to defer:
//
//    func runSubstep() (err error) {
//        s := Substep("Doing something...")
//        defer s.Finish(&err)
//
//        ...
//    }
//
func (s *substep) Finish(err *error) {
	status := idl.StepStatus_COMPLETE
	if *err != nil {
		status = idl.StepStatus_FAILED
	}

	fmt.Printf("%s\n", Format(s.description, status))
}
