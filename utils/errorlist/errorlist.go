// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package errorlist

import (
	"fmt"
	"strings"
)

// Append takes at least two errors and combines them into a list. The rules are
// as follows:
//
//   - If all of the errors being appended are nil, Append returns nil.
//   - If exactly one of the errors being appended is non-nil, Append returns
//     that error unchanged.
//   - Otherwise, Append returns a slice of errors (wrapped in an Errors
//     instance) consisting of the inputs, minus any nil errors.
//
// If any of the passed errors are themselves Errors slices, their contents will
// be flattened into the resulting error. This behavior can be prevented by
// wrapping input Errors slices in their own context, for example using
// fmt.Errorf.
func Append(a, b error, e ...error) error {
	errs := append([]error{a, b}, e...)

	var all Errors
	for _, err := range errs {
		switch v := err.(type) {
		case nil:
			continue
		case Errors:
			all = append(all, v...)
		default:
			all = append(all, v)
		}
	}

	switch len(all) {
	case 0:
		return nil
	case 1:
		return all[0]
	default:
		return all
	}
}

// Errors is a slice of error values that can be treated like a single error
// value. Use Append to create Errors instances.
type Errors []error

func (e Errors) Len() int {
	return len(e)
}

func (e Errors) Less(i, j int) bool {
	return e[i].Error() < e[j].Error()
}

func (e Errors) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}

func (e Errors) Error() string {
	if len(e) == 1 {
		return fmt.Sprintf("1 error occurred:\n\t* %s\n\n", e[0])
	}

	errors := make([]string, len(e))
	for i, err := range e {
		errors[i] = fmt.Sprintf("* %s", err)
	}

	return fmt.Sprintf(
		"%d errors occurred:\n\t%s\n\n",
		len(e), strings.Join(errors, "\n\t"))
}
