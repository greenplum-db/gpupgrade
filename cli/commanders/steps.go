// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commanders

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/xerrors"
	"google.golang.org/grpc/status"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

type receiver interface {
	Recv() (*idl.Message, error)
}

var indicators = map[idl.Status]string{
	idl.Status_RUNNING:  "[IN PROGRESS]",
	idl.Status_COMPLETE: "[COMPLETE]",
	idl.Status_FAILED:   "[FAILED]",
	idl.Status_SKIPPED:  "[SKIPPED]",
}

func Initialize(client idl.CliToHubClient, request *idl.InitializeRequest, verbose bool) (err error) {
	stream, err := client.Initialize(context.Background(), request)
	if err != nil {
		return err
	}

	_, err = UILoop(stream, verbose)
	if err != nil {
		return err
	}

	return nil
}

func InitializeCreateCluster(client idl.CliToHubClient, request *idl.InitializeCreateClusterRequest, verbose bool) (idl.InitializeResponse, error) {
	stream, err := client.InitializeCreateCluster(context.Background(), request)
	if err != nil {
		return idl.InitializeResponse{}, err
	}

	response, err := UILoop(stream, verbose)
	if err != nil {
		return idl.InitializeResponse{}, err
	}

	initializeResponse := response.GetInitializeResponse()
	if initializeResponse == nil {
		return idl.InitializeResponse{}, xerrors.Errorf("Initialize response is nil")
	}

	return *initializeResponse, nil
}

func Execute(client idl.CliToHubClient, verbose bool) (idl.ExecuteResponse, error) {
	stream, err := client.Execute(context.Background(), &idl.ExecuteRequest{})
	if err != nil {
		return idl.ExecuteResponse{}, err
	}

	response, err := UILoop(stream, verbose)
	if err != nil {
		return idl.ExecuteResponse{}, err
	}

	executeResponse := response.GetExecuteResponse()
	if executeResponse == nil {
		return idl.ExecuteResponse{}, xerrors.Errorf("Execute response is nil")
	}

	return *executeResponse, nil
}

func Finalize(client idl.CliToHubClient, verbose bool) (idl.FinalizeResponse, error) {
	stream, err := client.Finalize(context.Background(), &idl.FinalizeRequest{})
	if err != nil {
		return idl.FinalizeResponse{}, err
	}

	response, err := UILoop(stream, verbose)
	if err != nil {
		return idl.FinalizeResponse{}, err
	}

	finalizeResponse := response.GetFinalizeResponse()
	if finalizeResponse == nil {
		return idl.FinalizeResponse{}, xerrors.Errorf("Finalize response is nil")
	}

	return *finalizeResponse, nil
}

func Revert(client idl.CliToHubClient, verbose bool) (idl.RevertResponse, error) {
	stream, err := client.Revert(context.Background(), &idl.RevertRequest{})
	if err != nil {
		return idl.RevertResponse{}, err
	}

	response, err := UILoop(stream, verbose)
	if err != nil {
		return idl.RevertResponse{}, err
	}

	revertResponse := response.GetRevertResponse()
	if revertResponse == nil {
		return idl.RevertResponse{}, xerrors.Errorf("Revert response is nil")
	}

	return *revertResponse, nil
}

func UILoop(stream receiver, verbose bool) (*idl.Response, error) {
	var response *idl.Response
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
			response = x.Response

		default:
			panic(fmt.Sprintf("unknown message type: %T", x))
		}
	}

	if !verbose {
		fmt.Println()
	}

	if err != io.EOF {
		statusErr, ok := status.FromError(err)
		if !ok || len(statusErr.Details()) == 0 {
			return response, err
		}

		var nextActions []string
		for _, detail := range statusErr.Details() {
			if msg, ok := detail.(*idl.NextActions); ok {
				nextActions = append(nextActions, msg.GetNextActions())
			}
		}

		return response, utils.NewNextActionErr(err, strings.Join(nextActions, "\n"))
	}

	return response, nil
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
