// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commanders_test

import (
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/blang/semver/v4"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
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
				Type:   idl.Chunk_stdout,
			}}},
			{Contents: &idl.Message_Chunk{Chunk: &idl.Chunk{
				Buffer: []byte("my error"),
				Type:   idl.Chunk_stderr,
			}}},
			{Contents: &idl.Message_Chunk{Chunk: &idl.Chunk{
				Buffer: []byte("my string2"),
				Type:   idl.Chunk_stdout,
			}}},
		}

		d := BufferStandardDescriptors(t)
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
		var nextActionsErr utils.NextActionErr
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
		var nextActionsErr utils.NextActionErr
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
				Step:   idl.Substep_init_target_cluster,
				Status: idl.Status_running,
			}}},
			{Contents: &idl.Message_Chunk{Chunk: &idl.Chunk{
				Buffer: []byte("my string\n"),
				Type:   idl.Chunk_stdout,
			}}},
			{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_init_target_cluster,
				Status: idl.Status_complete,
			}}},
			{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_copy_master,
				Status: idl.Status_skipped,
			}}},
			{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_upgrade_master,
				Status: idl.Status_failed,
			}}},
		}

		expected := commanders.FormatStatus(msgs[0].GetStatus()) + "\n"
		expected += "my string\n"
		expected += commanders.FormatStatus(msgs[2].GetStatus()) + "\n"
		expected += commanders.FormatStatus(msgs[3].GetStatus()) + "\n"
		expected += commanders.FormatStatus(msgs[4].GetStatus()) + "\n"

		d := BufferStandardDescriptors(t)
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
				Step:   idl.Substep_init_target_cluster,
				Status: idl.Status_running,
			}}},
			{Contents: &idl.Message_Chunk{Chunk: &idl.Chunk{
				Buffer: []byte("output ignored"),
				Type:   idl.Chunk_stdout,
			}}},
			{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_init_target_cluster,
				Status: idl.Status_complete,
			}}},
			{Contents: &idl.Message_Chunk{Chunk: &idl.Chunk{
				Buffer: []byte("error ignored"),
				Type:   idl.Chunk_stderr,
			}}},
			{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_upgrade_master,
				Status: idl.Status_failed,
			}}},
		}

		// We expect output only from the status messages.
		expected := commanders.FormatStatus(msgs[0].GetStatus()) + "\r"
		expected += commanders.FormatStatus(msgs[2].GetStatus()) + "\n"
		expected += commanders.FormatStatus(msgs[4].GetStatus()) + "\n"

		d := BufferStandardDescriptors(t)
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
		source := MustCreateCluster(t, greenplum.SegConfigs{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole, Port: 15432},
			{ContentID: -1, DbID: 8, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: greenplum.MirrorRole, Port: 16432},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg0", Role: greenplum.PrimaryRole, Port: 25432},
			{ContentID: 0, DbID: 5, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg0", Role: greenplum.MirrorRole, Port: 25435},
		})
		source.GPHome = "/usr/local/greenplum-db-source"
		source.Destination = idl.ClusterDestination_source
		source.Version = semver.MustParse("5.0.0")

		target := MustCreateCluster(t, greenplum.SegConfigs{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole, Port: 6000},
			{ContentID: -1, DbID: 8, Hostname: "smdw", DataDir: "/data/qddir/seg-1", Role: greenplum.MirrorRole, Port: 6001},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg0", Role: greenplum.PrimaryRole, Port: 6002},
			{ContentID: 0, DbID: 5, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg0", Role: greenplum.MirrorRole, Port: 6005},
		})
		target.GPHome = "/usr/local/greenplum-db-target"
		target.Destination = idl.ClusterDestination_target
		target.Version = semver.MustParse("6.0.0")

		cases := []struct {
			name          string
			msgs          msgStream
			asertResponse func(response *idl.Response)
		}{
			{
				name: "processes initialize response successfully",
				msgs: msgStream{&idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{
					Contents: &idl.Response_InitializeResponse{InitializeResponse: &idl.InitializeResponse{
						HasAllMirrorsAndStandby: true,
					}}}}}},
				asertResponse: func(response *idl.Response) {
					actual := response.GetInitializeResponse().GetHasAllMirrorsAndStandby()
					if actual != true {
						t.Errorf("got %+v want %v", actual, true)
					}
				},
			},
			{
				name: "processes execute response successfully",
				msgs: msgStream{&idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{
					Contents: &idl.Response_ExecuteResponse{ExecuteResponse: &idl.ExecuteResponse{
						Intermediate: MustEncodeCluster(t, target),
					}}}}}},
				asertResponse: func(response *idl.Response) {
					decodedTarget := MustDecodeCluster(t, response.GetExecuteResponse().GetIntermediate())
					if !reflect.DeepEqual(target, decodedTarget) {
						t.Errorf("got %+v want %v", decodedTarget, target)
					}
				},
			},
			{
				name: "processes finalize response successfully",
				msgs: msgStream{&idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{
					Contents: &idl.Response_FinalizeResponse{FinalizeResponse: &idl.FinalizeResponse{
						Target:                                 MustEncodeCluster(t, target),
						LogArchiveDirectory:                    "/log/archive/dir",
						ArchivedSourceCoordinatorDataDirectory: "/archive/source/dir",
						UpgradeID:                              "ABC123",
					}}}}}},
				asertResponse: func(response *idl.Response) {
					decodedTarget := MustDecodeCluster(t, response.GetFinalizeResponse().GetTarget())
					if !reflect.DeepEqual(target, decodedTarget) {
						t.Errorf("got %+v want %v", decodedTarget, target)
					}

					expected := "/log/archive/dir"
					actual := response.GetFinalizeResponse().GetLogArchiveDirectory()
					if actual != expected {
						t.Errorf("got %+v want %v", actual, expected)
					}

					expected = "/archive/source/dir"
					actual = response.GetFinalizeResponse().GetArchivedSourceCoordinatorDataDirectory()
					if actual != expected {
						t.Errorf("got %+v want %v", actual, expected)
					}

					expected = "ABC123"
					actual = response.GetFinalizeResponse().GetUpgradeID()
					if actual != expected {
						t.Errorf("got %+v want %v", actual, expected)
					}
				},
			},
			{
				name: "processes revert response successfully",
				msgs: msgStream{&idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{
					Contents: &idl.Response_RevertResponse{RevertResponse: &idl.RevertResponse{
						Source:              MustEncodeCluster(t, source),
						LogArchiveDirectory: "/log/archive/dir",
					}}}}}},
				asertResponse: func(response *idl.Response) {
					decodedSource := MustDecodeCluster(t, response.GetRevertResponse().GetSource())
					if !reflect.DeepEqual(source, decodedSource) {
						t.Errorf("got %+v want %v", decodedSource, target)
					}

					expected := "/log/archive/dir"
					if response.GetRevertResponse().GetLogArchiveDirectory() != expected {
						t.Errorf("got %+v want %v", response.GetRevertResponse().GetLogArchiveDirectory(), expected)
					}
				},
			},
		}

		for _, c := range cases {
			response, err := commanders.UILoop(&c.msgs, false)
			if err != nil {
				t.Errorf("got unexpected err %+v", err)
			}

			c.asertResponse(response)
		}
	})

	t.Run("panics with unexpected protobuf messages", func(t *testing.T) {
		cases := []struct {
			name string
			msg  *idl.Message
		}{{
			"bad step",
			&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_unknown_substep,
				Status: idl.Status_complete,
			}}},
		}, {
			"bad status",
			&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_copy_master,
				Status: idl.Status_unknown_status,
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
