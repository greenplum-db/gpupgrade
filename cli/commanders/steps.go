// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commanders

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils/stopwatch"
)

type ExecuteResponse struct {
	TargetPort          string
	TargetMasterDataDir string
}

type FinalizeResponse struct {
	TargetPort          string
	TargetMasterDataDir string
}

type RevertResponse struct {
	SourcePort          string
	SourceMasterDataDir string
	Version             string
	ArchiveDir          string
}

type receiver interface {
	Recv() (*idl.Message, error)
}

type substepText struct {
	OutputText string
	HelpText   string
}

type substep struct {
	name    idl.Substep
	text    substepText
	verbose bool
	timer   *stopwatch.Stopwatch
}

var SubstepDescriptions = map[idl.Substep]substepText{
	idl.Substep_CREATING_DIRECTORIES:                     substepText{"Creating directories...", "Create directories"},
	idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG:             substepText{"Saving source cluster configuration...", "Save source cluster configuration"},
	idl.Substep_START_HUB:                                substepText{"Starting gpupgrade hub process...", "Start gpupgrade hub process"},
	idl.Substep_START_AGENTS:                             substepText{"Starting gpupgrade agent processes...", "Start gpupgrade agent processes"},
	idl.Substep_CHECK_DISK_SPACE:                         substepText{"Checking disk space...", "Check disk space"},
	idl.Substep_CREATE_TARGET_CONFIG:                     substepText{"Generating target cluster configuration...", "Generate target cluster configuration"},
	idl.Substep_INIT_TARGET_CLUSTER:                      substepText{"Creating target cluster...", "Create target cluster"},
	idl.Substep_SHUTDOWN_TARGET_CLUSTER:                  substepText{"Stopping target cluster...", "Stop target cluster"},
	idl.Substep_BACKUP_TARGET_MASTER:                     substepText{"Backing up target master...", "Back up target master"},
	idl.Substep_CHECK_UPGRADE:                            substepText{"Running pg_upgrade checks...", "Run pg_upgrade checks"},
	idl.Substep_SHUTDOWN_SOURCE_CLUSTER:                  substepText{"Stopping source cluster...", "Stop source cluster"},
	idl.Substep_UPGRADE_MASTER:                           substepText{"Upgrading master...", "Upgrade master"},
	idl.Substep_COPY_MASTER:                              substepText{"Copying master catalog to primary segments...", "Copy master catalog to primary segments"},
	idl.Substep_UPGRADE_PRIMARIES:                        substepText{"Upgrading primary segments...", "Upgrade primary segments"},
	idl.Substep_START_TARGET_CLUSTER:                     substepText{"Starting target cluster...", "Start target cluster"},
	idl.Substep_UPDATE_TARGET_CATALOG_AND_CLUSTER_CONFIG: substepText{"Updating target master catalog...", "Update target master catalog"},
	idl.Substep_UPDATE_DATA_DIRECTORIES:                  substepText{"Updating data directories...", "Update data directories"},
	idl.Substep_UPDATE_TARGET_CONF_FILES:                 substepText{"Updating target master configuration files...", "Update target master configuration files"},
	idl.Substep_UPGRADE_STANDBY:                          substepText{"Upgrading standby master...", "Upgrade standby master"},
	idl.Substep_UPGRADE_MIRRORS:                          substepText{"Upgrading mirror segments...", "Upgrade mirror segments"},
	idl.Substep_DELETE_TABLESPACES:                       substepText{"Deleting target tablespace directories...", "Delete target tablespace directories"},
	idl.Substep_DELETE_PRIMARY_DATADIRS:                  substepText{"Deleting primary segment data directories...", "Delete primary segment data directories"},
	idl.Substep_DELETE_MASTER_DATADIR:                    substepText{"Deleting master data directory...", "Delete master data directory"},
	idl.Substep_DELETE_SEGMENT_STATEDIRS:                 substepText{"Deleting state directories on the segments...", "Delete state directories on the segments"},
	idl.Substep_STOP_HUB_AND_AGENTS:                      substepText{"Stopping hub and agents...", "Stop hub and agents"},
	idl.Substep_DELETE_MASTER_STATEDIR:                   substepText{"Deleting master state directory...", "Delete master state directory"},
	idl.Substep_ARCHIVE_LOG_DIRECTORIES:                  substepText{"Archiving log directories...", "Archive log directories"},
	idl.Substep_RESTORE_SOURCE_CLUSTER:                   substepText{"Restoring source cluster...", "Restore source cluster"},
	idl.Substep_START_SOURCE_CLUSTER:                     substepText{"Starting source cluster...", "Start source cluster"},
	idl.Substep_RESTORE_PGCONTROL:                        substepText{"Re-enabling source cluster...", "Re-enable source cluster"},
	idl.Substep_RECOVERSEG_SOURCE_CLUSTER:                substepText{"Recovering source cluster mirrors...", "Recover source cluster mirrors"},
}

var indicators = map[idl.Status]string{
	idl.Status_RUNNING:  "[IN PROGRESS]",
	idl.Status_COMPLETE: "[COMPLETE]",
	idl.Status_FAILED:   "[FAILED]",
}

func Initialize(client idl.CliToHubClient, request *idl.InitializeRequest, verbose bool) (err error) {
	stream, err := client.Initialize(context.Background(), request)
	if err != nil {
		return xerrors.Errorf("initialize hub: %w", err)
	}

	_, err = UILoop(stream, verbose)
	if err != nil {
		return xerrors.Errorf("Initialize: %w", err)
	}

	return nil
}

func InitializeCreateCluster(client idl.CliToHubClient, verbose bool) (err error) {
	stream, err := client.InitializeCreateCluster(context.Background(),
		&idl.InitializeCreateClusterRequest{},
	)
	if err != nil {
		return xerrors.Errorf("initialize create cluster: %w", err)
	}

	_, err = UILoop(stream, verbose)
	if err != nil {
		return xerrors.Errorf("InitializeCreateCluster: %w", err)
	}

	return nil
}

func Execute(client idl.CliToHubClient, verbose bool) (*ExecuteResponse, error) {
	fmt.Println()
	fmt.Println("Execute in progress.")
	fmt.Println()

	stream, err := client.Execute(context.Background(), &idl.ExecuteRequest{})
	if err != nil {
		// TODO: Change the logging message?
		gplog.Error("ERROR - Unable to connect to hub")
		return &ExecuteResponse{}, err
	}

	response, err := UILoop(stream, verbose)
	if err != nil {
		return &ExecuteResponse{}, xerrors.Errorf("Execute: %w", err)
	}

	return &ExecuteResponse{
		TargetPort:          response[idl.ResponseKey_target_port.String()],
		TargetMasterDataDir: response[idl.ResponseKey_target_master_data_directory.String()],
	}, nil
}

func Finalize(client idl.CliToHubClient, verbose bool) (*FinalizeResponse, error) {
	fmt.Println()
	fmt.Println("Finalize in progress.")
	fmt.Println()

	stream, err := client.Finalize(context.Background(), &idl.FinalizeRequest{})
	if err != nil {
		gplog.Error(err.Error())
		return &FinalizeResponse{}, err
	}

	response, err := UILoop(stream, verbose)
	if err != nil {
		return &FinalizeResponse{}, xerrors.Errorf("Finalize: %w", err)
	}

	return &FinalizeResponse{
			TargetPort:          response[idl.ResponseKey_target_port.String()],
			TargetMasterDataDir: response[idl.ResponseKey_target_master_data_directory.String()],
		},
		nil
}

func Revert(client idl.CliToHubClient, verbose bool) (*RevertResponse, error) {
	fmt.Println()
	fmt.Println("Revert in progress.")
	fmt.Println()

	stream, err := client.Revert(context.Background(), &idl.RevertRequest{})
	if err != nil {
		gplog.Error(err.Error())
		return &RevertResponse{}, err
	}

	response, err := UILoop(stream, verbose)
	if err != nil {
		return &RevertResponse{}, xerrors.Errorf("Revert: %w", err)
	}

	return &RevertResponse{
		SourcePort:          response[idl.ResponseKey_source_port.String()],
		SourceMasterDataDir: response[idl.ResponseKey_source_master_data_directory.String()],
		Version:             response[idl.ResponseKey_source_version.String()],
		ArchiveDir:          response[idl.ResponseKey_revert_log_archive_directory.String()],
	}, nil
}

func UILoop(stream receiver, verbose bool) (map[string]string, error) {
	data := make(map[string]string)
	var lastStep idl.Substep
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
				if lastStep == idl.Substep_UNKNOWN_SUBSTEP {
					// This is the first call, so we don't need to "terminate"
					// the previous line at all.
				} else if x.Status.Step == lastStep {
					fmt.Print("\r")
				} else {
					fmt.Println()
				}
			}
			lastStep = x.Status.Step

			fmt.Print(FormatStatus(x.Status))
			if verbose {
				fmt.Println()
			}

		case *idl.Message_Response:
			// NOTE: the latest message will clobber earlier keys
			for k, v := range x.Response.Data {
				data[k] = v
			}

		default:
			panic(fmt.Sprintf("unknown message type: %T", x))
		}
	}

	if !verbose {
		fmt.Println()
	}

	if err != io.EOF {
		return data, err
	}

	return data, nil
}

// FormatStatus returns a status string based on the upgrade status message.
// It's exported for ease of testing.
//
// FormatStatus panics if it doesn't have a string representation for a given
// protobuf code.
func FormatStatus(status *idl.SubstepStatus) string {
	line, ok := SubstepDescriptions[status.Step]
	if !ok {
		panic(fmt.Sprintf("unexpected step %#v", status.Step))
	}

	return Format(line.OutputText, status.Status)
}

// Format is also exported for ease of testing (see FormatStatus). Use NewSubstep
// instead.
func Format(description string, status idl.Status) string {
	indicator, ok := indicators[status]
	if !ok {
		panic(fmt.Sprintf("unexpected status %#v", status))
	}

	return fmt.Sprintf("%-67s%-13s", description, indicator)
}

// NewSubstep prints out an "in progress" marker for the given substep description,
// and returns a struct that can be .Finish()d (in a defer statement) to print
// the final complete/failed state.
func NewSubstep(step idl.Substep, verbose bool) *substep {
	substepText := SubstepDescriptions[step]
	fmt.Printf("%s\r", Format(substepText.OutputText, idl.Status_RUNNING))

	return &substep{
		name:    step,
		text:    substepText,
		verbose: verbose,
		timer:   stopwatch.Start(),
	}
}

// Finish prints out the final status of the substep; either COMPLETE or FAILED
// depending on whether or not there is an error. The method takes a pointer to
// error rather than error to make it possible to defer:
//
//    func runSubstep() (err error) {
//        s := NewSubstep("Doing something...")
//        defer s.Finish(&err)
//
//        ...
//    }
//
func (s *substep) Finish(err *error) {
	status := idl.Status_COMPLETE
	if *err != nil {
		status = idl.Status_FAILED
	}

	fmt.Printf("%s\n", Format(s.text.OutputText, status))

	LogDuration(s.name.String(), s.verbose, s.timer.Stop())
}

func LogDuration(operation string, verbose bool, timer *stopwatch.Stopwatch) {
	msg := operation + " took " + timer.String()
	if verbose {
		fmt.Println("\n" + msg)
	}
	gplog.Debug(msg)
}
