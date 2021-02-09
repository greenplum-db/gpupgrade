//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package syncbuf

import (
	"bytes"
	"sync"
)

// Syncbuf implements a go-routine safe unbounded buffer
type Syncbuf struct {
	buf bytes.Buffer
	sync.Mutex
}

func New() *Syncbuf {
	return &Syncbuf{}
}

func (s *Syncbuf) Write(p []byte) (n int, err error) {
	s.Lock()
	defer s.Unlock()

	return s.buf.Write(p)
}

func (s *Syncbuf) Read(b []byte) (n int, err error) {
	s.Lock()
	defer s.Unlock()

	return s.buf.Read(b)
}

// Bytes() returns the current contents of the Syncbuf, without
// modifying it.
func (s *Syncbuf) Bytes() []byte {
	s.Lock()
	defer s.Unlock()

	// copy contents into a buffer that does not expose our internals
	var buf bytes.Buffer
	buf.Write(s.buf.Bytes())

	return buf.Bytes()
}
