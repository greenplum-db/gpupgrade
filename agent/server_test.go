package agent_test

import (
	"os"
	"path"
	"testing"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/testutils"
)

func TestServerStart(t *testing.T) {
	testhelper.SetupTestLogger()

	t.Run("successfully starts and creates state directory if it does not exist", func(t *testing.T) {
		tempDir := getTempDir(t)
		defer os.RemoveAll(tempDir)
		stateDir := path.Join(tempDir, ".gpupgrade")

		server := agent.NewServer(agent.Config{
			Port:     getOpenPort(t),
			StateDir: stateDir,
		})

		if pathExists(stateDir) {
			t.Fatal("expected stateDir to not exist")
		}

		go server.Start()
		defer server.Stop()

		exists, err := doesPathEventuallyExist(stateDir)
		if err != nil {
			t.Fatalf("unexpected error: %#v", err)
		}
		if !exists {
			t.Error("expected stateDir to be created")
		}
	})

	t.Run("successfully starts if state directory already exists", func(t *testing.T) {
		stateDir := getTempDir(t)
		defer os.RemoveAll(stateDir)

		server := agent.NewServer(agent.Config{
			Port:     getOpenPort(t),
			StateDir: stateDir,
		})

		if !pathExists(stateDir) {
			t.Fatal("expected stateDir to exist")
		}

		go server.Start()
		defer server.Stop()

		if !pathExists(stateDir) {
			t.Error("expected stateDir to exist")
		}
	})
}

func getOpenPort(t *testing.T) int {
	port, err := testutils.GetOpenPort()
	if err != nil {
		t.Fatalf("getting open port: %+v", err)
	}

	return port
}

func doesPathEventuallyExist(path string) (bool, error) {
	startTime := time.Now()
	timeout := 3 * time.Second

	for {
		exists := pathExists(path)
		if exists {
			return true, nil
		}

		if time.Since(startTime) > timeout {
			return false, xerrors.Errorf("timeout exceeded")
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return true
}
