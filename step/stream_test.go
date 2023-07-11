// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package step

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
)

// Since we have no good way to test devNullStream, we instead
// provide an example.
func ExampleDevNullStream() {
	const (
		stdout = "this command has progress..."
		stderr = "there are some warnings..."
	)
	stream := DevNullStream
	fmt.Fprintf(stream.Stdout(), "%s", stdout)
	fmt.Fprintf(stream.Stderr(), "%s", stderr)
	// Output:
}

func TestBufStream(t *testing.T) {
	t.Run("records stdout and stderr to the sender", func(t *testing.T) {
		const (
			stdout = "this command has progress..."
			stderr = "there are some warnings..."
		)

		stream := new(BufferedStreams)
		fmt.Fprintf(stream.Stdout(), "%s", stdout)
		fmt.Fprintf(stream.Stderr(), "%s", stderr)

		if stdout != stream.StdoutBuf.String() {
			t.Errorf("expected %s got %s", stdout, stream.StdoutBuf.String())
		}
		if stderr != stream.StderrBuf.String() {
			t.Errorf("expected %s got %s", stderr, stream.StderrBuf.String())
		}
	})
}

func TestMultiplexedStream(t *testing.T) {
	logOutput := testlog.SetupTestLogger()

	t.Run("forwards stdout and stderr to the sender", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		const (
			expectedStdout = "expected\nstdout\n"
			expectedStderr = "process\nstderr\n"
		)

		mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		mockStream.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Chunk{Chunk: &idl.Chunk{
				Buffer: []byte(expectedStdout),
				Type:   idl.Chunk_stdout,
			}}})
		mockStream.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Chunk{Chunk: &idl.Chunk{
				Buffer: []byte(expectedStderr),
				Type:   idl.Chunk_stderr,
			}}})

		stream := newLogMessageSender(mockStream)
		fmt.Fprint(stream.Stdout(), expectedStdout)
		fmt.Fprint(stream.Stderr(), expectedStderr)
	})

	t.Run("also writes all data to the log", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		mockStream.EXPECT().
			Send(gomock.Any()).
			AnyTimes()

		stream := newLogMessageSender(mockStream)

		// Write 10 bytes to each stream.
		for i := 0; i < 10; i++ {
			_, err := stream.Stdout().Write([]byte{'O'})
			if err != nil {
				t.Errorf("writing stdout: %#v", err)
			}
			_, err = stream.Stderr().Write([]byte{'E'})
			if err != nil {
				t.Errorf("writing stderr: %#v", err)
			}
		}

		logContents := string(logOutput.Bytes())
		expected := "OEOEOEOEOEOEOEOEOEOE"
		for _, char := range expected {
			if strings.HasSuffix(logContents, string(char)) {
				t.Errorf("log got %q, want %q", logContents, expected)
			}
		}
	})

	t.Run("continues writing to the log file even if Send fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Return an error during Send.
		mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		mockStream.EXPECT().
			Send(gomock.Any()).
			Return(errors.New("error during send")).
			Times(1) // we expect only one failed attempt to Send

		stream := newLogMessageSender(mockStream)

		// Write 10 bytes to each stream.
		for i := 0; i < 10; i++ {
			_, err := stream.Stdout().Write([]byte{'O'})
			if err != nil {
				t.Errorf("unexpected error %#v", err)
			}

			_, err = stream.Stderr().Write([]byte{'E'})
			if err != nil {
				t.Errorf("unexpected error %#v", err)
			}
		}

		// The Writer should not have been affected in any way.
		logContents := string(logOutput.Bytes())
		numO := strings.Count(logContents, "O\n")
		if numO != 20 {
			t.Errorf("got %d want 20", numO)
		}

		numE := strings.Count(logContents, "E\n")
		if numE != 20 {
			t.Errorf("got %d want 20", +numE)
		}

		expected := "halting client sender: error during send"
		if !strings.Contains(logContents, expected) {
			t.Errorf("log %q does not contain %q", logContents, expected)
		}
	})
}
