package db

import (
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
