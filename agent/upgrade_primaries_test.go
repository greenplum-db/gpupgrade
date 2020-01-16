package agent

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"
)

// Does nothing.
func EmptyMain() {}

// Prints the environment, one variable per line, in NAME=VALUE format.
func EnvironmentMain() {
	for _, e := range os.Environ() {
		fmt.Println(e)
	}
}

func FailedMain() {
	os.Exit(1)
}

func init() {
	exectest.RegisterMains(
		EmptyMain,
		EnvironmentMain,
		FailedMain,
	)
}

func TestUpgradePrimary(t *testing.T) {
	// Disable exec.Command. This way, if a test forgets to mock it out, we
	// crash the test instead of executing code on a dev system.
	execCommand = nil

	// We need a real temporary directory to change to. Replace MkdirAll() so
	// that we can make sure the directory is the correct one.
	tempDir, err := ioutil.TempDir("", "gpupgrade")
	if err != nil {
		t.Fatalf("creating temporary directory: %+v", err)
	}
	defer os.RemoveAll(tempDir)

	utils.System.MkdirAll = func(path string, perms os.FileMode) error {
		// Bail out if the implementation tries to touch any other directories.
		if !strings.HasPrefix(path, tempDir) {
			t.Fatalf("requested directory %q is not under temporary directory %q; refusing to create it",
				path, tempDir)
		}

		return os.MkdirAll(path, perms)
	}
	defer func() {
		utils.System = utils.InitializeSystemFunctions()
	}()

	pairs := []*idl.DataDirPair{
		{
			OldDataDir: "/data/old",
			NewDataDir: "/data/new",
			OldPort:    15432,
			NewPort:    15433,
			Content:    1,
			DBID:       2,
		},
		// TODO add a second pair when we can run multiple execCommand
		// invocations in a single test
	}

	// NOTE: we could choose to duplicate the upgrade.Run unit tests for all of
	// this, but we choose to instead rely on end-to-end tests for most of this
	// functionality, and test only a few integration paths here.

	t.Run("when pg_upgrade --check fails it returns an error", func(t *testing.T) {
		execCommand = exectest.NewCommand(FailedMain)
		defer func() { execCommand = nil }()

		err := UpgradePrimary("/old/bin", "/new/bin", pairs, tempDir, true, false)
		if err == nil {
			t.Fatal("UpgradeSegments() returned no error")
		}

		// XXX it'd be nice if we didn't couple against a hardcoded string here,
		// but it's difficult to unwrap multierror with the new xerrors
		// interface.
		if !strings.Contains(err.Error(), "failed to check primary on host") ||
			!strings.Contains(err.Error(), "with content 1") {
			t.Errorf("error %q did not contain expected contents 'check primary on host' and 'content 1'",
				err.Error())
		}
	})

	t.Run("when pg_upgrade with no check fails it returns an error", func(t *testing.T) {
		execCommand = exectest.NewCommand(FailedMain)
		defer func() { execCommand = nil }()

		err := UpgradePrimary("/old/bin", "/new/bin", pairs, tempDir, false, false)
		if err == nil {
			t.Fatal("UpgradeSegments() returned no error")
		}

		// XXX it'd be nice if we didn't couple against a hardcoded string here,
		// but it's difficult to unwrap multierror with the new xerrors
		// interface.
		if !strings.Contains(err.Error(), "failed to upgrade primary on host") ||
			!strings.Contains(err.Error(), "with content 1") {
			t.Errorf("error %q did not contain expected contents 'upgrade primary on host' and 'content 1'",
				err.Error())
		}
	})
}
