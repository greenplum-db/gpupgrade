package services

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

type ExecuteStream struct {
	stream messageSender
	log *os.File
}

func (h *Hub) Execute(request *idl.ExecuteRequest, stream idl.CliToHub_ExecuteServer) (err error) {
	// Create a log file to contain execute output.
	log, err := utils.System.OpenFile(
		filepath.Join(utils.GetStateDir(), "execute.log"),
		os.O_WRONLY|os.O_CREATE,
		0600,
	)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := log.Close(); closeErr != nil {
			err = multierror.Append(err,
				xerrors.Errorf("failed to close execute log: %w", closeErr))
		}
	}()

	executeStream := &ExecuteStream{stream: stream, log:log}

	_, err = log.WriteString("\nExecute in progress.\n")
	if err != nil {
		return xerrors.Errorf("failed writing to execute log: %w", err)
	}

	err = h.ExecuteSubStep(executeStream, upgradestatus.INIT_TARGET_CLUSTER, h.CreateTargetCluster)
	if err != nil {
		return err
	}

	err = h.ExecuteSubStep(executeStream, upgradestatus.SHUTDOWN_CLUSTERS, h.ShutdownClusters)
	if err != nil {
		return err
	}

	err = h.ExecuteSubStep(executeStream, upgradestatus.UPGRADE_MASTER, h.UpgradeMaster)
	if err != nil {
		return err
	}

	err = h.ExecuteSubStep(executeStream, upgradestatus.COPY_MASTER, h.CopyMasterDataDir)
	if err != nil {
		return err
	}

	err = h.ExecuteUpgradePrimariesSubStep(stream)
	if err != nil {
		return err
	}

	err = h.ExecuteSubStep(executeStream, upgradestatus.START_TARGET_CLUSTER, h.StartTargetCluster)
	return err
}

func (h *Hub) ExecuteSubStep(executeStream *ExecuteStream, subStep string, subStepFunc func(stream messageSender, log io.Writer) error) error {
	gplog.Info("starting %s", subStep)
	_, err := executeStream.log.WriteString(fmt.Sprintf("\nStarting %s...\n\n", subStep))
	if err != nil {
		return xerrors.Errorf("failed writing to execute log: %w", err)
	}

	step, err := h.InitializeStep(subStep, executeStream.stream)
	if err != nil {
		gplog.Error(err.Error())
		return err
	}

	err = subStepFunc(executeStream.stream, executeStream.log)
	if err != nil {
		gplog.Error(err.Error())
		step.MarkFailed()
	} else {
		step.MarkComplete()
	}

	return nil
}

// multiplexedStream provides io.Writers that wrap both gRPC stream and a parallel
// io.Writer (in case the gRPC stream closes) and safely serialize any
// simultaneous writes.
type multiplexedStream struct {
	stream messageSender
	writer io.Writer
	mutex  sync.Mutex
}

func newMultiplexedStream(stream messageSender, writer io.Writer) *multiplexedStream {
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
