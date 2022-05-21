// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package logger

import (
	"log"
	"runtime/debug"
)

// WritePanics is a deferrable helper function that will log a DEBUG stack trace
// if a panic is encountered. It then re-panics with the recovered value.
func WritePanics() {
	if r := recover(); r != nil {
		// Why not log.Printf()? Because we're going to re-panic, and there's
		// no need to spam the terminal twice. log.Printf() will push the
		// errors to the log without writing again to the standard streams.
		log.Printf("encountered panic (%#v); stack trace follows:\n%s", r, debug.Stack())

		panic(r)
	}
}
