// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"runtime/debug"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

// WritePanics is a deferrable helper function that will log a DEBUG stack trace
// if a panic is encountered. It then re-panics with the recovered value.
func WritePanics() {
	if r := recover(); r != nil {
		// Why not gplog.Error()? Because we're going to re-panic, and there's
		// no need to spam the terminal twice. gplog.Debug() will push the
		// errors to the log without writing again to the standard streams.
		gplog.Debug("encountered panic (%#v); stack trace follows:\n%s", r, debug.Stack())

		panic(r)
	}
}
