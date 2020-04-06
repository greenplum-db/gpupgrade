package agent_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/testutils"
)

func TestIsPostgres(t *testing.T) {
	testhelper.SetupTestLogger()

	t.Run("returns false if dir does not exist", func(t *testing.T) {
		if agent.IsPostgres("/does/not/exist/ABC1234") {
			t.Errorf("expected false")
		}
	})

	t.Run("returns true if required files exist", func(t *testing.T) {
		source, _, tmpDir := testutils.SetupDataDirs(t)
		defer func() {
			os.RemoveAll(tmpDir)
		}()

		if !agent.IsPostgres(source) {
			t.Errorf("expected true")
		}
	})

	t.Run("returns false if one required file is missing", func(t *testing.T) {
		source, _, tmpDir := testutils.SetupDataDirs(t)
		defer func() {
			os.RemoveAll(tmpDir)
		}()

		if err := os.Remove(filepath.Join(source, "postgresql.conf")); err != nil {
			t.Errorf("unexpected error %v", err)
		}

		if agent.IsPostgres(source) {
			t.Errorf("expected false")
		}
	})

}
