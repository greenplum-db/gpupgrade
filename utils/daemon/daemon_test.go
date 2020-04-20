// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package daemon

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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

var _ = Describe("waitForDaemon", func() {
	outbuf := new(bytes.Buffer)
	errbuf := new(bytes.Buffer)

	BeforeEach(func() {
		outbuf.Reset()
		errbuf.Reset()
	})

	It("starts the passed command", func() {
		cmd := MockDaemonizableCommand{}
		err := waitForDaemon(&cmd, outbuf, errbuf, 0)

		Expect(err).NotTo(HaveOccurred())
		Expect(cmd.Started).To(BeTrue())
	})

	It("does not wait for the command to complete if there is no stderr", func() {
		cmd := MockDaemonizableCommand{}
		err := waitForDaemon(&cmd, outbuf, errbuf, 0)

		Expect(err).NotTo(HaveOccurred())
		Expect(cmd.Waited).To(BeFalse())
	})

	It("waits for the command to complete if stderr contents are found", func() {
		errput := []byte("this is an error\n")
		exitErr := fmt.Errorf("process exited with code 1")
		cmd := MockDaemonizableCommand{StderrBuf: errput, WaitError: exitErr}

		err := waitForDaemon(&cmd, outbuf, errbuf, 0)
		Expect(err).To(Equal(exitErr))
		Expect(cmd.Waited).To(BeTrue())
	})

	It("does not start the command if standard pipes cannot be created", func() {
		pipeErr := fmt.Errorf("generic failure")
		cmds := [...]MockDaemonizableCommand{
			{StdoutErrorOnPipe: pipeErr},
			{StderrErrorOnPipe: pipeErr},
		}

		for i, cmd := range cmds {
			err := waitForDaemon(&cmd, outbuf, errbuf, 0)
			Expect(err).To(Equal(pipeErr), "in iteration %d:", i)
			Expect(cmd.Started).To(BeFalse(), "in iteration %d:", i)
		}
	})

	It("returns an error if the command cannot be started", func() {
		startErr := fmt.Errorf("start failure")
		cmd := MockDaemonizableCommand{StartError: startErr}

		err := waitForDaemon(&cmd, outbuf, errbuf, 0)
		Expect(err).To(Equal(startErr))
	})

	It("passes through pipe content from the child", func() {
		output := []byte("this is output\n")
		errput := []byte("this is an error\n")

		cmd := MockDaemonizableCommand{StdoutBuf: output, StderrBuf: errput}
		err := waitForDaemon(&cmd, outbuf, errbuf, 0)
		Expect(err).NotTo(HaveOccurred())

		Expect(outbuf.Bytes()).To(Equal(output))
		Expect(errbuf.Bytes()).To(Equal(errput))
	})

	It("errors out if it cannot copy from child pipe", func() {
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

			Expect(err).To(HaveOccurred(), "in iteration %d:", i)
			Expect(err.Error()).To(ContainSubstring(copyErrs[i].Error()), "in iteration %d:", i)
		}
	})

	It("times out if an error is reported but the command does not exit", func() {
		errput := []byte("this is an error\n")
		cmd := MockDaemonizableCommand{StderrBuf: errput, Hang: true}
		err := waitForDaemon(&cmd, outbuf, errbuf, 1*time.Millisecond)

		Expect(cmd.Waited).To(BeTrue())
		Expect(errbuf.Bytes()).To(Equal(errput))
		Expect(err).To(HaveOccurred())
	})

	It("does not time out if the timeout is set to zero", func() {
		errput := []byte("this is an error\n")
		cmd := MockDaemonizableCommand{StderrBuf: errput, Hang: true}

		c := make(chan error, 1)
		go func() {
			// waitForDaemon() should NOT return to write to the channel.
			c <- waitForDaemon(&cmd, outbuf, errbuf, 0)
		}()

		Consistently(c).ShouldNot(Receive())
		Expect(cmd.Waited).To(BeTrue())
	})
})
