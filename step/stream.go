// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package step

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"os"
	"strings"
	"sync"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
)

type OutStreams interface {
	Stdout() io.Writer
	Stderr() io.Writer
}

// DevNullStream provides an implementation of OutStreams that drops
// all writes to it.
var DevNullStream = devNullStream{}

type devNullStream struct{}

func (_ devNullStream) Stdout() io.Writer {
	return io.Discard
}

func (_ devNullStream) Stderr() io.Writer {
	return io.Discard
}

// BufferedStreams provides an implementation of OutStreams that stores
// all writes to underlying bytes.Buffer objects.
type BufferedStreams struct {
	StdoutBuf bytes.Buffer
	StderrBuf bytes.Buffer
}

func (s *BufferedStreams) Stdout() io.Writer {
	return &s.StdoutBuf
}

func (s *BufferedStreams) Stderr() io.Writer {
	return &s.StderrBuf
}

// StdStreams implements OutStreams that writes directly to stdout and stderr
var StdStreams = &stdStreams{}

type stdStreams struct{}

func (m *stdStreams) Stdout() io.Writer {
	return os.Stdout
}

func (m *stdStreams) Stderr() io.Writer {
	return os.Stderr
}

// logMessageSender is a type of OutStreams that writes to both a gRPC MessageSender
// and a log file. Writing to stdout/stderr will also write to the log file.
type logMessageSender struct {
	stdout io.Writer
	stderr io.Writer

	sender idl.MessageSender
	mutex  sync.Mutex
}

func newLogMessageSender(stream idl.MessageSender) *logMessageSender {
	lms := &logMessageSender{sender: stream}

	lms.stdout = &logMessageSenderWriter{
		logMessageSender: lms,
		cType:            idl.Chunk_stdout,
	}

	lms.stderr = &logMessageSenderWriter{
		logMessageSender: lms,
		cType:            idl.Chunk_stderr,
	}

	return lms
}

func (l *logMessageSender) Stdout() io.Writer {
	return l.stdout
}

func (l *logMessageSender) Stderr() io.Writer {
	return l.stderr
}

// logMessageSenderWriter is an internal type used by logMessageSender to send stdout and
// stderr to both a gRPC MessageSender and log file.
type logMessageSenderWriter struct {
	*logMessageSender
	cType idl.Chunk_Type
}

func (l *logMessageSenderWriter) Write(p []byte) (int, error) {
	l.logMessageSender.mutex.Lock()
	defer l.logMessageSender.mutex.Unlock()

	scanner := bufio.NewScanner(bytes.NewReader(p))
	for scanner.Scan() {
		line := scanner.Text()
		log.Print(strings.TrimSpace(line)) // avoid awkward newlines in the log file
	}

	if err := scanner.Err(); err != nil {
		return 0, xerrors.Errorf("scanning: %w", err)
	}

	if l.logMessageSender.sender != nil {
		// Attempt to send the chunk to the client. Since the client may close
		// the connection at any point, errors here are logged and otherwise
		// ignored. After the first send error, no more attempts are made.

		chunk := &idl.Chunk{
			Buffer: p,
			Type:   l.cType,
		}

		err := l.logMessageSender.sender.Send(&idl.Message{
			Contents: &idl.Message_Chunk{Chunk: chunk},
		})

		if err != nil {
			log.Printf("halting client sender: %v", err)
			l.logMessageSender.sender = nil
		}
	}

	return len(p), nil
}
