// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commanders_test

import (
	"errors"
	"io"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/greenplum-db/gpupgrade/cli"
	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
)

type msgStream []*idl.Message

func (m *msgStream) Recv() (*idl.Message, error) {
	if len(*m) == 0 {
		return nil, io.EOF
	}

	// This looks a little weird. It's just dequeuing from the front of the
	// slice.
	nextMsg := (*m)[0]
	*m = (*m)[1:]

	return nextMsg, nil
}

type errStream struct {
	err error
}

func (m *errStream) Recv() (*idl.Message, error) {
	return nil, m.err
}

func TestUILoop(t *testing.T) {
	t.Run("writes STDOUT and STDERR chunks in the order they are received", func(t *testing.T) {
		msgs := msgStream{
			{Contents: &idl.Message_Chunk{Chunk: &idl.Chunk{
				Buffer: []byte("my string1"),
				Type:   idl.Chunk_STDOUT,
			}}},
			{Contents: &idl.Message_Chunk{Chunk: &idl.Chunk{
				Buffer: []byte("my error"),
				Type:   idl.Chunk_STDERR,
			}}},
			{Contents: &idl.Message_Chunk{Chunk: &idl.Chunk{
				Buffer: []byte("my string2"),
				Type:   idl.Chunk_STDOUT,
			}}},
		}

		d := commanders.BufferStandardDescriptors(t)
		defer d.Close()

		_, err := commanders.UILoop(&msgs, true)
		if err != nil {
			t.Errorf("UILoop() returned %#v", err)
		}

		actualOut, actualErr := d.Collect()

		actual, expected := string(actualOut), "my string1my string2"
		if actual != expected {
			t.Errorf("stdout was %#v want %#v", actual, expected)
		}

		actual, expected = string(actualErr), "my error"
		if actual != expected {
			t.Errorf("stderr was %#v want %#v", actual, expected)
		}
	})

	t.Run("returns an error when a non io.EOF error is encountered", func(t *testing.T) {
		expected := errors.New("bengie")

		_, err := commanders.UILoop(&errStream{expected}, true)
		if err != expected {
			t.Errorf("returned %#v want %#v", err, expected)
		}
	})

	t.Run("returns next action when error contains next action in details", func(t *testing.T) {
		expected := "do these next actions"
		statusErr := status.New(codes.Internal, "oops")
		statusErr, err := statusErr.WithDetails(&idl.NextActions{NextActions: expected})
		if err != nil {
			t.Fatal("failed to add next action details")
		}

		_, err = commanders.UILoop(&errStream{statusErr.Err()}, true)
		var nextActionsErr cli.NextActions
		if !errors.As(err, &nextActionsErr) {
			t.Errorf("got type %T want %T", err, nextActionsErr)
		}

		expectedErr := "rpc error: code = Internal desc = oops"
		if err.Error() != expectedErr {
			t.Errorf("got error %#v want %#v", err.Error(), expectedErr)
		}

		if nextActionsErr.NextAction != expected {
			t.Fatalf("got %q want %q", nextActionsErr.NextAction, expected)
		}
	})

	t.Run("does not return a next action status error has no details", func(t *testing.T) {
		statusErr := status.New(codes.Internal, "oops")
		_, err := commanders.UILoop(&errStream{statusErr.Err()}, true)
		var nextActionsErr cli.NextActions
		if errors.As(err, &nextActionsErr) {
			t.Errorf("got type %T do not want %T", err, nextActionsErr)
		}

		expectedErr := "rpc error: code = Internal desc = oops"
		if err.Error() != expectedErr {
			t.Errorf("got error %#v want %#v", err.Error(), expectedErr)
		}
	})

	t.Run("writes status and stdout chunks serially in verbose mode", func(t *testing.T) {
		msgs := msgStream{
			{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_INIT_TARGET_CLUSTER,
				Status: idl.Status_RUNNING,
			}}},
			{Contents: &idl.Message_Chunk{Chunk: &idl.Chunk{
				Buffer: []byte("my string\n"),
				Type:   idl.Chunk_STDOUT,
			}}},
			{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_INIT_TARGET_CLUSTER,
				Status: idl.Status_COMPLETE,
			}}},
			{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_COPY_MASTER,
				Status: idl.Status_SKIPPED,
			}}},
			{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_UPGRADE_MASTER,
				Status: idl.Status_FAILED,
			}}},
		}

		expected := commanders.FormatStatus(msgs[0].GetStatus()) + "\n"
		expected += "my string\n"
		expected += commanders.FormatStatus(msgs[2].GetStatus()) + "\n"
		expected += commanders.FormatStatus(msgs[3].GetStatus()) + "\n"
		expected += commanders.FormatStatus(msgs[4].GetStatus()) + "\n"

		d := commanders.BufferStandardDescriptors(t)
		defer d.Close()

		_, err := commanders.UILoop(&msgs, true)
		if err != nil {
			t.Errorf("UILoop() returned %#v", err)
		}

		actualOut, actualErr := d.Collect()

		if len(actualErr) != 0 {
			t.Errorf("unexpected stderr %#v", string(actualErr))
		}

		actual := string(actualOut)
		if actual != expected {
			t.Errorf("output %#v want %#v", actual, expected)
		}
	})

	t.Run("overwrites status lines and ignores chunks in non-verbose mode", func(t *testing.T) {
		msgs := msgStream{
			{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_INIT_TARGET_CLUSTER,
				Status: idl.Status_RUNNING,
			}}},
			{Contents: &idl.Message_Chunk{Chunk: &idl.Chunk{
				Buffer: []byte("output ignored"),
				Type:   idl.Chunk_STDOUT,
			}}},
			{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_INIT_TARGET_CLUSTER,
				Status: idl.Status_COMPLETE,
			}}},
			{Contents: &idl.Message_Chunk{Chunk: &idl.Chunk{
				Buffer: []byte("error ignored"),
				Type:   idl.Chunk_STDERR,
			}}},
			{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_UPGRADE_MASTER,
				Status: idl.Status_FAILED,
			}}},
		}

		// We expect output only from the status messages.
		expected := commanders.FormatStatus(msgs[0].GetStatus()) + "\r"
		expected += commanders.FormatStatus(msgs[2].GetStatus()) + "\n"
		expected += commanders.FormatStatus(msgs[4].GetStatus()) + "\n"

		d := commanders.BufferStandardDescriptors(t)
		defer d.Close()

		_, err := commanders.UILoop(&msgs, false)
		if err != nil {
			t.Errorf("UILoop() returned %#v", err)
		}

		actualOut, actualErr := d.Collect()

		if len(actualErr) != 0 {
			t.Errorf("unexpected stderr %#v", string(actualErr))
		}

		actual := string(actualOut)
		if actual != expected {
			t.Errorf("output %#v want %#v", actual, expected)
		}
	})

	t.Run("processes responses successfully", func(t *testing.T) {
		cases := []struct {
			name     string
			msgs     msgStream
			expected func(response *idl.Response) bool
		}{
			{
				name: "processes initialize response successfully",
				msgs: msgStream{&idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{
					Contents: &idl.Response_InitializeResponse{InitializeResponse: &idl.InitializeResponse{
						HasMirrors: true,
					}}}}}},
				expected: func(response *idl.Response) bool {
					return response.GetInitializeResponse().GetHasMirrors() == true &&
						response.GetInitializeResponse().GetHasStandby() == false
				},
			},
			{
				name: "processes execute response successfully",
				msgs: msgStream{&idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{
					Contents: &idl.Response_ExecuteResponse{ExecuteResponse: &idl.ExecuteResponse{
						Target: &idl.Cluster{
							Port:                15423,
							MasterDataDirectory: "/data/gpseg-1",
						}}}}}}},
				expected: func(response *idl.Response) bool {
					return response.GetExecuteResponse().GetTarget().GetPort() == 15423 &&
						response.GetExecuteResponse().GetTarget().GetMasterDataDirectory() == "/data/gpseg-1"
				},
			},
			{
				name: "processes finalize response successfully",
				msgs: msgStream{&idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{
					Contents: &idl.Response_FinalizeResponse{FinalizeResponse: &idl.FinalizeResponse{
						Target: &idl.Cluster{
							Port:                15433,
							MasterDataDirectory: "/data/gpseg-10",
						}}}}}}},
				expected: func(response *idl.Response) bool {
					return response.GetFinalizeResponse().GetTarget().GetPort() == 15433 &&
						response.GetFinalizeResponse().GetTarget().GetMasterDataDirectory() == "/data/gpseg-10"
				},
			},
			{
				name: "processes revert response successfully",
				msgs: msgStream{&idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{
					Contents: &idl.Response_RevertResponse{RevertResponse: &idl.RevertResponse{
						Source: &idl.Cluster{
							Port:                1111,
							MasterDataDirectory: "/data/gpseg-2",
						},
						SourceVersion:       "5.0",
						LogArchiveDirectory: "/gpAdminLogs/1112",
					}}}}}},
				expected: func(response *idl.Response) bool {
					return response.GetRevertResponse().GetSource().GetPort() == 1111 &&
						response.GetRevertResponse().GetSource().GetMasterDataDirectory() == "/data/gpseg-2" &&
						response.GetRevertResponse().GetSourceVersion() == "5.0" &&
						response.GetRevertResponse().GetLogArchiveDirectory() == "/gpAdminLogs/1112"
				},
			},
		}

		for _, c := range cases {
			actual, err := commanders.UILoop(&c.msgs, false)
			if err != nil {
				t.Errorf("got unexpected err %+v", err)
			}

			if !c.expected(actual) {
				t.Errorf("got unexpected response %s", actual)
			}
		}
	})

	t.Run("panics with unexpected protobuf messages", func(t *testing.T) {
		cases := []struct {
			name string
			msg  *idl.Message
		}{{
			"bad step",
			&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_UNKNOWN_SUBSTEP,
				Status: idl.Status_COMPLETE,
			}}},
		}, {
			"bad status",
			&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_COPY_MASTER,
				Status: idl.Status_UNKNOWN_STATUS,
			}}},
		}, {
			"bad message type",
			&idl.Message{Contents: nil},
		}}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("did not panic")
					}
				}()

				msgs := &msgStream{c.msg}
				_, err := commanders.UILoop(msgs, false)
				if err != nil {
					t.Fatalf("got error %q want panic", err)
				}
			})
		}
	})
}

func TestFormatStatus(t *testing.T) {
	t.Run("it formats all possible types", func(t *testing.T) {
		ignoreUnknownStep := 1
		ignoreInternalStepStatus := 1
		numberOfSubsteps := len(idl.Substep_name) - ignoreUnknownStep - ignoreInternalStepStatus

		if numberOfSubsteps != len(commanders.SubstepDescriptions) {
			t.Errorf("got %q, expected FormatStatus to be able to format all %d statuses %q. Formatted only %d",
				commanders.SubstepDescriptions, len(idl.Substep_name), idl.Substep_name, len(commanders.SubstepDescriptions))
		}
	})
}
