//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package syncbuf_test

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/greenplum-db/gpupgrade/utils/syncbuf"
)

func TestSyncbuf(t *testing.T) {
	t.Run("writes to a buffer", func(t *testing.T) {
		s := syncbuf.New()

		expected := []byte("hello")
		_, err := s.Write(expected)
		if err != nil {
			t.Fatalf("Write returned error %+v", err)
		}

		actual := s.Bytes()
		if !bytes.Equal(actual, expected) {
			t.Errorf("got %#v expected %#v", actual, expected)
		}
	})

	t.Run("make sure Write is atomic", func(t *testing.T) {
		seq1 := []byte("01234")
		seq2 := []byte("56789")

		var wg sync.WaitGroup
		s := syncbuf.New()

		// 10 runs always fails when we remove synchronization from s.Write()
		runs := 10
		for run := 0; run < runs; run++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := s.Write(seq1)
				if err != nil {
					t.Errorf("Write failed %+v", err)
				}
				_, err = s.Write(seq2)
				if err != nil {
					t.Errorf("Write failed %+v", err)
				}
			}()
		}

		wg.Wait()

		// Check that seq1 or seq2 are written in any order the correct number of times.
		expectedLength := (len(seq1) + len(seq2)) * runs
		actual := make([]byte, expectedLength)
		_, err := s.Read(actual)
		if err != nil {
			t.Fatalf("Read returned error %+v", err)
		}

		numSeq1 := strings.Count(string(actual), string(seq1))
		if numSeq1 != runs {
			t.Errorf("found seq1 %d times want %d in %q", numSeq1, runs, actual)
		}

		numSeq2 := strings.Count(string(actual), string(seq2))
		if numSeq2 != runs {
			t.Errorf("found seq2 %d times want %d in %q", numSeq2, runs, actual)
		}
	})
}
