// Copyright (c) 2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package errorlist_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func TestAppend(t *testing.T) {
	t.Run("multiple nils appended together are still nil", func(t *testing.T) {
		cases := []struct {
			desc   string
			actual error
		}{
			{"nil, nil", errorlist.Append(nil, nil)},
			{"nil, nil, nil, nil", errorlist.Append(nil, nil, nil, nil)},
		}

		for _, c := range cases {
			if c.actual != nil {
				t.Errorf("Append(%s) = %#v, want nil", c.desc, c.actual)
			}
		}
	})

	t.Run("nil appended with one error is just the error itself", func(t *testing.T) {
		expected := errors.New("it broke")

		cases := []struct {
			desc   string
			actual error
		}{
			{"err, nil", errorlist.Append(expected, nil)},
			{"nil, err", errorlist.Append(nil, expected)},
			{"nil, nil, err, nil, nil", errorlist.Append(nil, nil, expected, nil, nil)},
		}

		for _, c := range cases {
			if c.actual != expected {
				t.Errorf("Append(%s) = %#v, want %#v", c.desc, c.actual, expected)
			}
		}
	})

	t.Run("two plain errors appended together results in an Errors list", func(t *testing.T) {
		first := errors.New("ahhh")
		second := errors.New("it broke")

		expected := errorlist.Errors{first, second}
		testlist(t, expected, first, second)
	})

	t.Run("an error appended to an Errors list gets added to the end", func(t *testing.T) {
		expected := errorlist.Errors{
			errors.New("ahhh"),
			errors.New("it broke"),
			errors.New("bad"),
		}

		first := errorlist.Errors{expected[0], expected[1]}
		second := expected[2]

		testlist(t, expected, first, second)
	})

	t.Run("Errors lists appended together are concatenated in order", func(t *testing.T) {
		expected := errorlist.Errors{
			errors.New("ahhh"),
			errors.New("it broke"),
			errors.New("bad"),
			errors.New("really bad"),
			errors.New("five"),
			errors.New("six"),
		}

		first := errorlist.Errors{expected[0], expected[1]}
		second := errorlist.Errors{expected[2], expected[3]}
		third := errorlist.Errors{expected[4], expected[5]}

		testlist(t, expected, first, second, third)
	})

	t.Run("wrapped Errors lists are not concatenated", func(t *testing.T) {
		expected := errorlist.Errors{
			errors.New("ahhh"),
			fmt.Errorf("context: %w", errorlist.Errors{
				errors.New("wrapped 1"),
				errors.New("wrapped 2"),
			}),
			errors.New("it broke"),
		}

		testlist(t, expected, expected[0], expected[1], expected[2])
	})
}

// testlist Append()s all input and tests that the result is equivalent to the
// expected Errors list.
func testlist(t *testing.T, expected errorlist.Errors, input ...error) {
	t.Helper()

	actual := errorlist.Append(input[0], input[1], input[2:]...)

	var errs errorlist.Errors
	if !errors.As(actual, &errs) {
		t.Fatalf("Append%q = %#v, want type %T", input, actual, errs)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("Append%q = %#v, want %#v", input, actual, expected)
	}
}

func TestErrors(t *testing.T) {
	t.Run("Error() displays multiple errors", func(t *testing.T) {
		errs := errorlist.Errors{
			fmt.Errorf("context: %w", errors.New("ahhh")),
			errors.New("it broke"),
			errors.New("bad"),
		}

		actual := errs.Error()
		expected := `3 errors occurred:
	* context: ahhh
	* it broke
	* bad

`

		if actual != expected {
			t.Errorf("Error() = %q, want %q", actual, expected)
		}
	})
}
