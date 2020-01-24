package hub

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/onsi/gomega/gbytes"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils"

	. "github.com/onsi/gomega"
)

func TestMultiplexedStream(t *testing.T) {
	// Store gplog output.
	_, _, log := testhelper.SetupTestLogger()

	t.Run("forwards stdout and stderr to the stream", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		const (
			expectedStdout = "expected\nstdout\n"
			expectedStderr = "process\nstderr\n"
		)

		mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		mockStream.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Chunk{&idl.Chunk{
				Buffer: []byte(expectedStdout),
				Type:   idl.Chunk_STDOUT,
			}}})
		mockStream.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Chunk{&idl.Chunk{
				Buffer: []byte(expectedStderr),
				Type:   idl.Chunk_STDERR,
			}}})

		stream := newMultiplexedStream(mockStream, ioutil.Discard)
		fmt.Fprint(stream.Stdout(), expectedStdout)
		fmt.Fprint(stream.Stderr(), expectedStderr)
	})

	t.Run("also writes all data to a local io.Writer", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		mockStream.EXPECT().
			Send(gomock.Any()).
			AnyTimes()

		var buf bytes.Buffer
		stream := newMultiplexedStream(mockStream, &buf)

		// Write 10 bytes to each stream.
		for i := 0; i < 10; i++ {
			stream.Stdout().Write([]byte{'O'})
			stream.Stderr().Write([]byte{'E'})
		}

		expected := "OEOEOEOEOEOEOEOEOEOE"
		if buf.String() != expected {
			t.Errorf("writer got %q, want %q", buf.String(), expected)
		}
	})

	t.Run("continues writing to the local io.Writer even if Send fails", func(t *testing.T) {
		g := NewGomegaWithT(t)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Return an error during Send.
		mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		mockStream.EXPECT().
			Send(gomock.Any()).
			Return(errors.New("error during send")).
			Times(1) // we expect only one failed attempt to Send

		var buf bytes.Buffer
		stream := newMultiplexedStream(mockStream, &buf)

		// Write 10 bytes to each stream.
		for i := 0; i < 10; i++ {
			stream.Stdout().Write([]byte{'O'})
			stream.Stderr().Write([]byte{'E'})
		}

		// The Writer should not have been affected in any way.
		g.Expect(buf.Bytes()).To(HaveLen(20))
		g.Expect(log).To(gbytes.Say("halting client stream: error during send"))
	})

	t.Run("bubbles up underlying io.Writer failures before streaming", func(t *testing.T) {
		expected := errors.New("ahhhh")

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		// we expect no calls on the stream

		stream := newMultiplexedStream(mockStream, &failingWriter{expected})

		_, err := stream.Stdout().Write([]byte{'x'})
		if !xerrors.Is(err, expected) {
			t.Errorf("Stdout().Write() returned %#v, want %#v", err, expected)
		}

		_, err = stream.Stderr().Write([]byte{'x'})
		if !xerrors.Is(err, expected) {
			t.Errorf("Stderr().Write() returned %#v, want %#v", err, expected)
		}
	})
}

func TestSubstep(t *testing.T) {
	testhelper.SetupTestLogger()

	cm := testutils.NewMockChecklistManager()
	hub := New(&Config{}, nil, "", cm)

	t.Run("sends execution status to checklist and client", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sender := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		stream := newMultiplexedStream(sender, ioutil.Discard)

		sender.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{&idl.SubstepStatus{
				Step:   idl.Substep_UNKNOWN_STEP,
				Status: idl.Status_RUNNING,
			}}})
		sender.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{&idl.SubstepStatus{
				Step:   idl.Substep_UNKNOWN_STEP,
				Status: idl.Status_COMPLETE,
			}}})

		var called bool
		step := "mystep"

		err := hub.Substep(stream, step,
			func(_ OutStreams) error {
				called = true

				// We shouldn't be marked complete until this returns.
				actual := cm.GetStepReader(step).Status()
				if actual != idl.Status_RUNNING {
					t.Errorf("step was marked %s, want %s", actual, idl.Status_RUNNING)
				}

				return nil
			})

		if err != nil {
			t.Errorf("returned error %#v", err)
		}
		if !called {
			t.Error("substep callback was not executed")
		}

		actual := cm.GetStepReader(step).Status()
		if actual != idl.Status_COMPLETE {
			t.Errorf("step was marked %s, want %s", actual, idl.Status_COMPLETE)
		}
	})

	t.Run("bubbles up errors from substep callbacks", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sender := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		stream := newMultiplexedStream(sender, ioutil.Discard)

		sender.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{&idl.SubstepStatus{
				Step:   idl.Substep_UNKNOWN_STEP,
				Status: idl.Status_RUNNING,
			}}})
		sender.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{&idl.SubstepStatus{
				Step:   idl.Substep_UNKNOWN_STEP,
				Status: idl.Status_FAILED,
			}}})

		step := "mystep"
		expected := errors.New("ahhhh")
		err := hub.Substep(stream, step,
			func(_ OutStreams) error {
				return expected
			})

		if !xerrors.Is(err, expected) {
			t.Errorf("returned %#v, want %#v", err, expected)
		}

		actual := cm.GetStepReader(step).Status()
		if actual != idl.Status_FAILED {
			t.Errorf("step was marked %s, want %s", actual, idl.Status_FAILED)
		}
	})
}
