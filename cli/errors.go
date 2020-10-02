// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package cli

import "fmt"

// NextActions attaches the PrintHelp method to an existing error. This is used
// to tell the CLI's top level to print additional helper text AFTER the error
// message is printed.
type NextActions struct {
	error
	Subcommand    string // the gpupgrade subcommand name to print
	suggestRevert bool
}

func NewNextActions(err error, subcommand string, suggestRevert bool) NextActions {
	return NextActions{
		error:         err,
		Subcommand:    subcommand,
		suggestRevert: suggestRevert,
	}
}

func (n NextActions) PrintHelp() {
	text := `
NEXT ACTIONS
------------
Please address the above issue and run "gpupgrade %s" again.
`
	if n.suggestRevert {
		text += `

If you would like to return the cluster to its original state, please run "gpupgrade revert".
`
	}
	fmt.Printf(text, n.Subcommand)
}
