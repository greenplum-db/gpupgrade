package hub

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/onsi/gomega/gbytes"
	"golang.org/x/xerrors"

	. "github.com/onsi/gomega"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
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

func TestStatusFile(t *testing.T) {
	stateDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(stateDir); err != nil {
			t.Errorf("removing temp directory: %v", err)
		}
	}()

	path := filepath.Join(stateDir, "status.json")

	t.Run("creates status file if it does not exist", func(t *testing.T) {
		_, err := os.Open(path)
		if !os.IsNotExist(err) {
			t.Errorf("returned error %#v want ErrNotExist", err)
		}

		statusFile, err := getStatusFile(stateDir)
		if err != nil {
			t.Errorf("unexpected error %v", err)
		}

		actual, err := ioutil.ReadFile(statusFile)
		if err != nil {
			t.Fatalf("ReadFile(%q) returned error %#v", statusFile, err)
		}

		expected := "{}"
		if string(actual) != expected {
			t.Errorf("read %v want %v", string(actual), expected)
		}
	})

	t.Run("does not create status file if it already exists", func(t *testing.T) {
		expected := "1234"
		err := ioutil.WriteFile(path, []byte(expected), 0600)
		if err != nil {
			t.Fatalf("unexpected error %v", err)
		}

		statusFile, err := getStatusFile(stateDir)
		if err != nil {
			t.Errorf("unexpected error %v", err)
		}

		actual, err := ioutil.ReadFile(statusFile)
		if err != nil {
			t.Errorf("ReadFile(%q) returned error %#v", statusFile, err)
		}

		if string(actual) != expected {
			t.Errorf("read %v want %v", string(actual), expected)
		}
	})
}
