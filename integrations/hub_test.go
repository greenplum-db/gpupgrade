package integrations_test

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/testutils"
)

const HubPort = 7527

func TestHub(t *testing.T) {
	t.Run("the hub does not daemonizes unless --deamonize is passed", func(t *testing.T) {
		killHub(t)
		defer killHub(t)

		dir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", filepath.Join(dir, ".gpupgrade"))
		defer resetEnv()

		err := commanders.CreateStateDir()
		if err != nil {
			t.Errorf("unexpected error got %#v", err)
		}

		err = commanders.CreateInitialClusterConfigs()
		if err != nil {
			t.Errorf("unexpected error got %#v", err)
		}

		cmd := exec.Command("gpupgrade", "hub")
		errChan := make(chan error, 1)

		go func() {
			errChan <- cmd.Run() // expected to never return
		}()

		select {
		case err := <-errChan:
			if err != nil {
				t.Errorf("unexpected error %#v", err)
			}
		case <-time.After(100 * time.Millisecond):
			// hub daemonizes without an error
		}
	})
}

// killHub finds all running hub processes and kills them.
// XXX we should really use a PID file for this, and allow side-by-side hubs,
// rather than blowing away developer state.
func killHub(t *testing.T) {
	t.Helper()

	defer func() {
		if isPortInUse(HubPort) {
			t.Errorf("hub port %d is not available", HubPort)
		}
	}()

	killCommand := exec.Command("pkill", "-f", "^gpupgrade hub")
	err := killCommand.Run()
	// pkill returns exit code 1 if no processes were matched, which is fine.
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() == 1 {
			return
		}
	}

	if err != nil {
		t.Errorf("unexpected error got %#v", err)
	}
}

func isPortInUse(port int) bool {
	t := time.After(2 * time.Second)
	select {
	case <-t:
		fmt.Println("timed out")
		break
	default:
		cmd := exec.Command("/bin/sh", "-c", "'lsof | grep "+strconv.Itoa(port)+"'")
		err := cmd.Run()
		output, _ := cmd.CombinedOutput()
		if _, ok := err.(*exec.ExitError); ok && string(output) == "" {
			return false
		}

		time.Sleep(250 * time.Millisecond)
	}

	return true
}
