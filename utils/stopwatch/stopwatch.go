// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package stopwatch

import "time"

type Stopwatch struct {
	startTime   time.Time
	elapsedTime time.Duration
	stopped     bool
}

func Start() *Stopwatch {
	return &Stopwatch{
		startTime: time.Now(),
	}
}

// NewTime is used for unit testing.
func NewTime(start time.Time) *Stopwatch {
	return &Stopwatch{
		startTime: start,
	}
}

func (s *Stopwatch) Stop() *Stopwatch {
	if !s.stopped {
		s.elapsedTime = time.Since(s.startTime)
		s.stopped = true
	}

	return s
}

func (s *Stopwatch) String() string {
	return round(s.elapsedTime).String()
}

// round returns a pretty-printable duration. This may omit precision and
// rounding to the next lowest unit.
func round(duration time.Duration) time.Duration {
	switch {
	case duration.Seconds() < 1:
		return duration.Round(time.Millisecond)

	case duration.Minutes() < 60:
		return duration.Round(time.Second)

	default:
		return duration.Round(time.Minute)
	}
}
