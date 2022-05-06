// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package testlog

import (
	"strings"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/utils/syncbuf"
)

// TODO: if no callers use testSdtout and testStderr we can simplify this interface.
func SetupLogger() (*syncbuf.Syncbuf, *syncbuf.Syncbuf, *syncbuf.Syncbuf) {
	testStdout := syncbuf.New()
	testStderr := syncbuf.New()
	testLogfile := syncbuf.New()
	testLogger := gplog.NewLogger(testStdout, testStderr, testLogfile, "sync.SyncBuf", gplog.LOGINFO, "testProgram")
	gplog.SetLogger(testLogger)
	return testStdout, testStderr, testLogfile
}

func VerifyLogContains(t *testing.T, testlog *syncbuf.Syncbuf, expected string) {
	t.Helper()
	verifyLog(t, testlog, expected, true)
}

func VerifyLogDoesNotContain(t *testing.T, testlog *syncbuf.Syncbuf, expected string) {
	t.Helper()
	verifyLog(t, testlog, expected, false)
}

func verifyLog(t *testing.T, testlog *syncbuf.Syncbuf, expected string, shouldContain bool) {
	t.Helper()

	contents := string(testlog.Bytes())
	contains := strings.Contains(contents, expected)
	if shouldContain && !contains {
		t.Errorf("\nexpected log: %q\nto contain:   %q", contents, expected)
	}

	if !shouldContain && contains {
		t.Errorf("\nexpected log: %q\nto not contain:   %q", contents, expected)
	}
}
