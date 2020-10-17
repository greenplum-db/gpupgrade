// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commanders_test

import (
	"errors"
	"io"
	"reflect"
	"testing"

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

	t.Run("returns a map of strings that are processed in the stream", func(t *testing.T) {
		firstMap := make(map[string]string)
		firstMap["a"] = "b"
		firstMap["e"] = "f"

		secondMap := make(map[string]string)
		secondMap["a"] = "c"
		secondMap["g"] = "5432"

		msgs := msgStream{
			{Contents: &idl.Message_Response{
				Response: &idl.Response{Data: firstMap},
			}},
			{Contents: &idl.Message_Response{
				Response: &idl.Response{Data: secondMap},
			}},
		}

		actual, err := commanders.UILoop(&msgs, false)

		if err != nil {
			t.Errorf("got unexpected err %+v", err)
		}

		expected := make(map[string]string)
		expected["a"] = "c"
		expected["e"] = "f"
		expected["g"] = "5432"

		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("got map %#v want %#v", actual, expected)
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
