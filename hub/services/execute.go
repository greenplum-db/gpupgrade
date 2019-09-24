package services

import (
	"io"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/idl"
)

func (h *Hub) Execute (request *idl.ExecuteRequest, stream idl.CliToHub_ExecuteServer) error {
	err := h.ExecuteInitTargetClusterSubStep(stream)
	if err != nil {
		return err
	}

	err = h.ExecuteShutdownClustersSubStep(stream)
	if err != nil {
		return err
	}

	err = h.ExecuteUpgradeMasterSubStep(stream)
	if err != nil {
		return err
	}

	err = h.ExecuteCopyMasterSubStep()
	if err != nil {
		return err
	}

	err = h.ExecuteUpgradePrimariesSubStep()
	if err != nil {
		return err
	}

	err = h.ExecuteStartTargetClusterSubStep(stream)
	return err
}

// multiplexedStream provides io.Writers that wrap both gRPC stream and a parallel
// io.Writer (in case the gRPC stream closes) and safely serialize any
// simultaneous writes.
type multiplexedStream struct {
	stream idl.CliToHub_ExecuteServer
	writer io.Writer
	mutex  sync.Mutex
}

func newMultiplexedStream(stream idl.CliToHub_ExecuteServer, writer io.Writer) *multiplexedStream {
	return &multiplexedStream{
		stream: stream,
		writer: writer,
	}
}

func (m *multiplexedStream) NewStreamWriter(cType idl.Chunk_Type) io.Writer {
	return &streamWriter{
		multiplexedStream: m,
		cType:             cType,
	}
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
		err = w.stream.Send(&idl.Chunk{
			Buffer: p,
			Type:   w.cType,
		})

		if err != nil {
			gplog.Info("halting client stream: %v", err)
			w.stream = nil
		}
	}

	return len(p), nil
}
