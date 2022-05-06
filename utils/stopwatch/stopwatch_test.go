// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package stopwatch_test

import (
	"testing"
	"time"

	"github.com/greenplum-db/gpupgrade/utils/stopwatch"
)

// The internal implementation calls round to minimize test flakes. For
// example, 3h26m8s rounds to 3h26m giving the test 8 seconds to complete.
func TestStopwatch(t *testing.T) {
	t.Run("outputs correctly formatted duration", func(t *testing.T) {
		startTime := mustParseDuration(t, "-3h26m8s")
		timer := stopwatch.NewTime(time.Now().Add(startTime))
		timer.Stop()

		expected := mustParseDuration(t, "3h26m")
		if timer.String() != expected.String() {
			t.Errorf("got %q want %q", timer.String(), expected)
		}
	})

	t.Run("correctly rounds duration", func(t *testing.T) {
		cases := []struct {
			duration time.Duration
			expected time.Duration
		}{
			{
				duration: mustParseDuration(t, "-31.80526130h"),
				expected: mustParseDuration(t, "31h48m"),
			},
			{
				duration: mustParseDuration(t, "-31.80526130m"),
				expected: mustParseDuration(t, "31m48s"),
			},
			{
				duration: mustParseDuration(t, "-31.80526130s"),
				expected: mustParseDuration(t, "32s"),
			},
			{
				duration: mustParseDuration(t, "-31.80526130ms"),
				expected: mustParseDuration(t, "32ms"),
			},
		}

		now := time.Now()
		for _, c := range cases {
			timer := stopwatch.NewTime(now.Add(c.duration))
			timer.Stop()

			if timer.String() != c.expected.String() {
				t.Errorf("got %s want %v", timer.String(), c.expected)
			}
		}
	})
}

func mustParseDuration(t *testing.T, input string) time.Duration {
	t.Helper()

	duration, err := time.ParseDuration(input)
	if err != nil {
		t.Fatalf("parsing duration %q: %v", input, err)
	}

	return duration
}
