// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package utils

// NextActionErr attaches the Help method to an existing error. This is used
// to tell the CLI's top level to print additional helper text AFTER the error
// message is printed.
type NextActionErr struct {
	Err        error
	NextAction string
}

func NewNextActionErr(err error, nextAction string) NextActionErr {
	return NextActionErr{
		Err:        err,
		NextAction: nextAction,
	}
}

func (n NextActionErr) Error() string {
	return n.Err.Error()
}

func (n NextActionErr) Help() string {
	return `
NEXT ACTIONS
------------
` + n.NextAction
}
