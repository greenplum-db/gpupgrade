// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package daemon

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

// basicReadCloser is a helper struct for the implementation of NewReadCloser.
type basicReadCloser struct {
	io.Reader
}

func (basicReadCloser) Close() error { return nil }

// NewReadCloser is a very simple wrapper around bytes.NewReader which returns
// an io.ReadCloser instead of an io.Reader. The Close() method on this struct
// does absolutely nothing.
func NewReadCloser(buf []byte) io.ReadCloser {
	return basicReadCloser{
		Reader: bytes.NewReader(buf),
	}
}

// errorReadCloser will error out after filling its buffer during Read().
type errorReadCloser struct {
	ReadErr error
}

func (errorReadCloser) Close() error { return nil }

func (e errorReadCloser) Read(p []byte) (int, error) {
	for i := 0; i < len(p); i++ {
		p[i] = byte('x')
	}
	return len(p), e.ReadErr
}

// MockDaemonizableCommand is an implementation of main.DaemonizableCommand that
// provides some test helpers.
type MockDaemonizableCommand struct {
	Started    bool
	StartError error
	Waited     bool
	WaitError  error

	StdoutBuf         []byte
	StdoutErrorOnPipe error
	StdoutErrorOnRead error
	StderrBuf         []byte
	StderrErrorOnPipe error
	StderrErrorOnRead error

	Hang bool
}

func (m *MockDaemonizableCommand) Start() error {
	m.Started = true
	return m.StartError
}

func (m *MockDaemonizableCommand) Wait() (err error) {
	m.Waited = true

	if m.Hang {
		select {}
	}

	return m.WaitError
}

func (m *MockDaemonizableCommand) StdoutPipe() (io.ReadCloser, error) {
	if m.StdoutErrorOnPipe != nil {
		return nil, m.StdoutErrorOnPipe
	}

	var reader io.ReadCloser
	if m.StdoutErrorOnRead != nil {
		reader = errorReadCloser{ReadErr: m.StdoutErrorOnRead}
	} else {
		reader = NewReadCloser(m.StdoutBuf)
	}
	return reader, nil
}

func (m *MockDaemonizableCommand) StderrPipe() (io.ReadCloser, error) {
	if m.StderrErrorOnPipe != nil {
		return nil, m.StderrErrorOnPipe
	}

	var reader io.ReadCloser
	if m.StderrErrorOnRead != nil {
		reader = errorReadCloser{ReadErr: m.StderrErrorOnRead}
	} else {
		reader = NewReadCloser(m.StderrBuf)
	}
	return reader, nil
}

func resetBuffers(outbuf, errbuf *bytes.Buffer) {
	outbuf.Reset()
	errbuf.Reset()
}

func TestWaitForDaemon(t *testing.T) {
	outbuf := new(bytes.Buffer)
	errbuf := new(bytes.Buffer)

	t.Run("starts the passed command", func(t *testing.T) {
		defer resetBuffers(outbuf, errbuf)

		cmd := MockDaemonizableCommand{}
		err := waitForDaemon(&cmd, outbuf, errbuf, 0)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}

		if cmd.Started != true {
			t.Errorf("got %t want %t", cmd.Started, true)
		}
	})

	t.Run("does not wait for the command to complete if there is no stderr", func(t *testing.T) {
		defer resetBuffers(outbuf, errbuf)

		cmd := MockDaemonizableCommand{}
		err := waitForDaemon(&cmd, outbuf, errbuf, 0)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}

		if cmd.Waited != false {
			t.Errorf("got %t want %t", cmd.Waited, false)
		}
	})

	t.Run("waits for the command to complete if stderr contents are found", func(t *testing.T) {
		defer resetBuffers(outbuf, errbuf)

		errput := []byte("this is an error\n")
		exitErr := fmt.Errorf("process exited with code 1")
		cmd := MockDaemonizableCommand{StderrBuf: errput, WaitError: exitErr}

		err := waitForDaemon(&cmd, outbuf, errbuf, 0)
		if !errors.Is(err, exitErr) {
			t.Errorf("returned error %#v want %#v", err, exitErr)
		}

		if cmd.Waited != true {
			t.Errorf("got %t want %t", cmd.Waited, true)
		}
	})

	t.Run("does not start the command if standard pipes cannot be created", func(t *testing.T) {
		defer resetBuffers(outbuf, errbuf)

		pipeErr := fmt.Errorf("generic failure")
		cmds := [...]MockDaemonizableCommand{
			{StdoutErrorOnPipe: pipeErr},
			{StderrErrorOnPipe: pipeErr},
		}

		for _, cmd := range cmds {
			err := waitForDaemon(&cmd, outbuf, errbuf, 0)
			if !errors.Is(err, pipeErr) {
				t.Errorf("returned error %#v want %#v", err, pipeErr)
			}

			if cmd.Started != false {
				t.Errorf("got %t want %t", cmd.Started, false)
			}
		}
	})

	t.Run("returns an error if the command cannot be started", func(t *testing.T) {
		defer resetBuffers(outbuf, errbuf)

		startErr := fmt.Errorf("start failure")
		cmd := MockDaemonizableCommand{StartError: startErr}

		err := waitForDaemon(&cmd, outbuf, errbuf, 0)
		if !errors.Is(err, startErr) {
			t.Errorf("returned error %#v want %#v", err, startErr)
		}
	})

	t.Run("passes through pipe content from the child", func(t *testing.T) {
		defer resetBuffers(outbuf, errbuf)

		output := "this is output\n"
		errput := "this is an error\n"

		cmd := MockDaemonizableCommand{StdoutBuf: []byte(output), StderrBuf: []byte(errput)}
		err := waitForDaemon(&cmd, outbuf, errbuf, 0)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}

		if outbuf.String() != output {
			t.Errorf("got %q want %q", outbuf.String(), output)
		}

		if errbuf.String() != errput {
			t.Errorf("got %q want %q", errbuf.String(), errput)
		}
	})

	t.Run("errors out if it cannot copy from child pipe", func(t *testing.T) {
		defer resetBuffers(outbuf, errbuf)

		copyErrs := [...]error{
			errors.New("copy error during stdout"),
			errors.New("copy error during stderr"),
		}
		cmds := [...]MockDaemonizableCommand{
			{StdoutErrorOnRead: copyErrs[0]},
			{StderrErrorOnRead: copyErrs[1]},
		}

		for i, cmd := range cmds {
			err := waitForDaemon(&cmd, outbuf, errbuf, 0)
			if err == nil {
				t.Errorf("expected error %#v got nil", err)
			}

			if !strings.Contains(err.Error(), copyErrs[i].Error()) {
				t.Errorf("expected error %#v to contain %q", err, copyErrs[i].Error())
			}
		}
	})

	t.Run("times out if an error is reported but the command does not exit", func(t *testing.T) {
		defer resetBuffers(outbuf, errbuf)

		errput := "this is an error\n"
		cmd := MockDaemonizableCommand{StderrBuf: []byte(errput), Hang: true}
		err := waitForDaemon(&cmd, outbuf, errbuf, 1*time.Millisecond)
		if err == nil {
			t.Errorf("expected error %#v got nil", err)
		}

		if errbuf.String() != errput {
			t.Errorf("got %q want %q", errbuf.String(), errput)
		}

		if cmd.Waited != true {
			t.Errorf("got %t want %t", cmd.Waited, true)
		}
	})

	t.Run("does not time out if the timeout is set to zero", func(t *testing.T) {
		defer resetBuffers(outbuf, errbuf)

		errput := []byte("this is an error\n")
		cmd := MockDaemonizableCommand{StderrBuf: errput, Hang: true}

		c := make(chan error, 1)
		go func() {
			// waitForDaemon() should NOT return to write to the channel.
			c <- waitForDaemon(&cmd, outbuf, errbuf, 0)
		}()

		select {
		case err := <-c:
			if err != nil {
				t.Errorf("unexpected error %#v", err)
			}
		case <-time.After(100 * time.Millisecond):
			// waitForDaemon without an error
		}

		if cmd.Waited != true {
			t.Errorf("got %t want %t", cmd.Waited, true)
		}
	})
}
