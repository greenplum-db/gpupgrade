// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package testutils

import (
	"io"
)

// DevNullWithClose implements step.OutStreams
type DevNullSpy struct {
	OutStream io.Writer
}

func (s DevNullSpy) Stdout() io.Writer {
	return s.OutStream
}

func (s DevNullSpy) Stderr() io.Writer {
	return io.Discard
}

// FailingStreams is an implementation of OutStreams for which every call to a
// stream's Write() method will fail with the given error.
type FailingStreams struct {
	Err error
}

func (f FailingStreams) Stdout() io.Writer {
	return &FailingWriter{f.Err}
}

func (f FailingStreams) Stderr() io.Writer {
	return &FailingWriter{f.Err}
}
