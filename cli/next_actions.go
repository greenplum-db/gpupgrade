// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package cli

import "fmt"

// NextActions attaches the PrintHelp method to an existing error. This is used
// to tell the CLI's top level to print additional helper text AFTER the error
// message is printed.
type NextActions struct {
	error
	NextAction string
}

func NewNextActions(err error, nextAction string) NextActions {
	return NextActions{
		error:      err,
		NextAction: nextAction,
	}
}

func (n NextActions) PrintHelp() {
	fmt.Print(`
NEXT ACTIONS
------------
` + n.NextAction)
}
