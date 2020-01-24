package hub

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/step"
)

func BeginStep(stateDir string, name string, sender idl.MessageSender) (*step.Step, error) {
	path := filepath.Join(stateDir, fmt.Sprintf("%s.log", name))
	log, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return nil, xerrors.Errorf(`step "%s": %w`, name, err)
	}

	_, err = fmt.Fprintf(log, "\n%s in progress.\n", strings.Title(name))
	if err != nil {
		log.Close()
		return nil, xerrors.Errorf(`logging step "%s": %w`, name, err)
	}

	statusPath, err := getStatusFile(stateDir)
	if err != nil {
		return nil, xerrors.Errorf("step %q: %w", name, err)
	}

	streams := newMultiplexedStream(sender, log)
	return step.New(name, sender, step.NewFileStore(statusPath), streams), nil
}

// Returns path to status file, and if one does not exist it creates an empty
// JSON file.
func getStatusFile(stateDir string) (path string, err error) {
	path = filepath.Join(stateDir, "status.json")

	f, err := os.OpenFile(path, os.O_EXCL|os.O_CREATE|os.O_WRONLY, 0600)
	if os.IsExist(err) {
		return path, nil
	}
	if err != nil {
		return "", err
	}

	defer func() {
		if cErr := f.Close(); cErr != nil {
			err = multierror.Append(err, cErr).ErrorOrNil()
		}
	}()

	// MarshallJSON requires a well-formed JSON file
	_, err = f.WriteString("{}")
	if err != nil {
		return "", err
	}

	return path, nil
}

// multiplexedStream provides an implementation of OutStreams that safely
// serializes any simultaneous writes to an underlying messageSender. A fallback
// io.Writer (in case the gRPC stream closes) also receives any output that is
// written to the streams.
type multiplexedStream struct {
	stream idl.MessageSender
	writer io.Writer
	mutex  sync.Mutex

	stdout io.Writer
	stderr io.Writer
}

func newMultiplexedStream(stream idl.MessageSender, writer io.Writer) *multiplexedStream {
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
