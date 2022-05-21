// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package logger_test

import (
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/utils/logger"
)

func TestWritePanics(t *testing.T) {
	t.Run("writes panics to the log file", func(t *testing.T) {
		buffer := testlog.SetupTestLogger()

		expected := "ahhh"
		defer func() {
			if e := recover(); e != nil {
				contents := string(buffer.Bytes())
				if !strings.Contains(contents, expected) {
					t.Errorf("expected %q in log file: %q", expected, contents)
				}
			}
		}()

		defer logger.WritePanics()
		panic(expected)
	})
}
