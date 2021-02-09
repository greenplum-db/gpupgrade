// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package db

import (
	"os"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils"
)

func TestNewDBConn(t *testing.T) {
	t.Run("uses environment variable when database parameter is empty", func(t *testing.T) {
		expected := "testdb"
		resetEnv := testutils.SetEnv(t, "PGDATABASE", expected)
		defer resetEnv()

		conn := NewDBConn("localHost", 5432, "")
		if conn.DBName != expected {
			t.Errorf("got database %q want %q", conn.DBName, expected)
		}
	})

	t.Run("uses database parameter when both parameter and environment variable are set", func(t *testing.T) {
		resetEnv := testutils.SetEnv(t, "PGDATABASE", "testdb")
		defer resetEnv()

		expected := "template1"
		conn := NewDBConn("localHost", 5432, expected)
		if conn.DBName != expected {
			t.Errorf("got database %q want %q", conn.DBName, expected)
		}
	})

	t.Run("uses environment variable when host parameter is empty", func(t *testing.T) {
		expected := "mdw"
		resetEnv := testutils.SetEnv(t, "PGHOST", expected)
		defer resetEnv()

		conn := NewDBConn("", 5432, "template1")
		if conn.Host != expected {
			t.Errorf("got host %q want %q", conn.DBName, expected)
		}
	})

	t.Run("uses host parameter when both parameter and environment variable are set", func(t *testing.T) {
		resetEnv := testutils.SetEnv(t, "PGHOST", "mdw")
		defer resetEnv()

		expected := "localhost"
		conn := NewDBConn(expected, 5432, "template1")
		if conn.Host != expected {
			t.Errorf("got host %q want %q", conn.DBName, expected)
		}
	})
}

func TestUserUtils(t *testing.T) {
	t.Run("tryEnv returns environment variables", func(t *testing.T) {
		expected := "val"

		resetEnv := testutils.SetEnv(t, "VAR", expected)
		defer resetEnv()

		actual := tryEnv("VAR", "default")
		if actual != expected {
			t.Errorf("got %q want %q", actual, expected)
		}
	})

	t.Run("tryEnv returns the default value when an environment variable does not exist", func(t *testing.T) {
		// ensure the variable is not set
		err := os.Unsetenv("VAR")
		if err != nil {
			t.Errorf("Unsetenv returend error %+v", err)
		}

		expected := "default"
		actual := tryEnv("VAR", expected)
		if actual != expected {
			t.Errorf("got %q want %q", actual, expected)
		}
	})
}
