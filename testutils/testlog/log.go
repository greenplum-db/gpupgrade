// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package testlog

import (
	"log"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/utils/syncbuf"
)

func SetupTestLogger() *syncbuf.Syncbuf {
	output := syncbuf.New()
	log.SetOutput(output)

	return output
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
