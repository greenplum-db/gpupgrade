package hub

import (
	"fmt"
	"io"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus/file"
	"github.com/greenplum-db/gpupgrade/idl"
)

// OutStreams collects the conceptual output and error streams into a single
// interface.
type OutStreams interface {
	Stdout() io.Writer
	Stderr() io.Writer
}

// Substep executes an upgrade substep of the given name using the provided
// implementation callback. All status and error reporting is coordinated on the
// provided stream.
func (h *Hub) Substep(stream *multiplexedStream, name string, f func(OutStreams) error) error {
	gplog.Info("starting %s", name)
	_, err := fmt.Fprintf(stream.writer, "\nStarting %s...\n\n", name)
	if err != nil {
		return xerrors.Errorf("failed writing to log: %w", err)
	}

	step, err := h.InitializeStep(name, stream.stream)
	if err != nil {
		gplog.Error(err.Error())
		return err
	}

	err = f(stream)
	if err != nil {
		gplog.Error(err.Error())
		step.MarkFailed()
	} else {
		step.MarkComplete()
	}

	return err
}

// Extracts common hub logic to reset state directory, mark step as in-progress,
// and control status streaming.
func (h *Hub) InitializeStep(step string, stream messageSender) (upgradestatus.StateWriter, error) {
	stepWriter := streamStepWriter{
		h.checklist.GetStepWriter(step),
		stream,
	}

	err := stepWriter.ResetStateDir()
	if err != nil {
		return nil, errors.Wrap(err, "failed to reset state directory")
	}

	err = stepWriter.MarkInProgress()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set %s to %s", step, file.InProgress)
	}

	return stepWriter, nil
}

// streamStepWriter extends the standard StepWriter, which only writes state to
// disk, with functionality that sends status updates across the given stream.
// (In practice this stream will be a gRPC CliToHub_XxxServer interface.)
type streamStepWriter struct {
	upgradestatus.StateWriter
	stream messageSender
}

type messageSender interface {
	Send(*idl.Message) error // matches gRPC streaming Send()
}

// TODO: remove; this is part of step.Step now
func sendStatus(stream messageSender, step idl.Substep, status idl.Status) {
	// A stream is not guaranteed to remain connected during execution, so
	// errors are explicitly ignored.
	_ = stream.Send(&idl.Message{
		Contents: &idl.Message_Status{&idl.SubstepStatus{
			Step:   step,
			Status: status,
		}},
	})
}

func (s streamStepWriter) MarkInProgress() error {
	if err := s.StateWriter.MarkInProgress(); err != nil {
		return err
	}

	sendStatus(s.stream, s.Code(), idl.Status_RUNNING)
	return nil
}

func (s streamStepWriter) MarkComplete() error {
	if err := s.StateWriter.MarkComplete(); err != nil {
		return err
	}

	sendStatus(s.stream, s.Code(), idl.Status_COMPLETE)
	return nil
}

func (s streamStepWriter) MarkFailed() error {
	if err := s.StateWriter.MarkFailed(); err != nil {
		return err
	}

	sendStatus(s.stream, s.Code(), idl.Status_FAILED)
	return nil
}

// multiplexedStream provides an implementation of OutStreams that safely
// serializes any simultaneous writes to an underlying messageSender. A fallback
// io.Writer (in case the gRPC stream closes) also receives any output that is
// written to the streams.
type multiplexedStream struct {
	stream messageSender
	writer io.Writer
	mutex  sync.Mutex

	stdout io.Writer
	stderr io.Writer
}

func newMultiplexedStream(stream messageSender, writer io.Writer) *multiplexedStream {
	m := &multiplexedStream{
		stream: stream,
		writer: writer,
	}

	m.stdout = &streamWriter{
		multiplexedStream: m,
		cType:             idl.Chunk_STDOUT,
	}
	m.stderr = &streamWriter{
		multiplexedStream: m,
		cType:             idl.Chunk_STDERR,
	}

	return m
}

func (m *multiplexedStream) Stdout() io.Writer {
	return m.stdout
}

func (m *multiplexedStream) Stderr() io.Writer {
	return m.stderr
}

// Close closes the stream's io.Writer if that writer also provides a Close
// method (i.e. it also implements io.WriteCloser). If not, Close is a no-op.
func (m *multiplexedStream) Close() error {
	if closer, ok := m.writer.(io.WriteCloser); ok {
		return closer.Close()
	}

	return nil
}

type streamWriter struct {
	*multiplexedStream
	cType idl.Chunk_Type
}

func (w *streamWriter) Write(p []byte) (int, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	n, err := w.writer.Write(p)
	if err != nil {
		return n, err
	}

	if w.stream != nil {
		// Attempt to send the chunk to the client. Since the client may close
		// the connection at any point, errors here are logged and otherwise
		// ignored. After the first send error, no more attempts are made.

		chunk := &idl.Chunk{
			Buffer: p,
			Type:   w.cType,
		}

		err = w.stream.Send(&idl.Message{
			Contents: &idl.Message_Chunk{chunk},
		})

		if err != nil {
			gplog.Info("halting client stream: %v", err)
			w.stream = nil
		}
	}

	return len(p), nil
}
