package commanders

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/greenplum-db/gpupgrade/utils"
)

func TestCreateInitialClusterConfigs(t *testing.T) {
	home, err := ioutil.TempDir("", t.Name())
	if err != nil {
		t.Fatalf("failed creating temp dir %#v", err)
	}

	oldStateDir, isSet := os.LookupEnv("GPUGRADE_HOME")
	defer func() {
		if isSet {
			os.Setenv("GPUPGRADE_HOME", oldStateDir)
		}
	}()
	stateDir := filepath.Join(home, ".gpupgrade")
	err = os.Setenv("GPUPGRADE_HOME", stateDir)
	if err != nil {
		t.Fatalf("failed to set GPUPGRADE_HOME %#v", err)
	}

	if _, err := os.Stat(stateDir); err == nil {
		t.Errorf("stateDir exists")
	}
	err = CreateStateDir()
	if err != nil {
		t.Fatalf("failed to create state dir %#v", err)
	}

	oldBinDir := "old/dir/bin"
	newBinDir := "new/dir/bin"
	var sourceOld, targetOld os.FileInfo

	t.Run("test idempotence", func(t *testing.T) {

		{ // creates initial cluster config files if none exist or fails"
			err = CreateInitialClusterConfigs(oldBinDir, newBinDir)
			if err != nil {
				t.Fatalf("unexpected error %#v", err)
			}

			if sourceOld, err = os.Stat(filepath.Join(stateDir, utils.SOURCE_CONFIG_FILENAME)); err != nil {
				t.Errorf("unexpected error %#v", err)
			}
			if targetOld, err = os.Stat(filepath.Join(stateDir, utils.TARGET_CONFIG_FILENAME)); err != nil {
				t.Errorf("unexpected error %#v", err)
			}
		}

		{ // creating cluster config files is idempotent
			err = CreateInitialClusterConfigs(oldBinDir, newBinDir)
			if err != nil {
				t.Fatalf("unexpected error %#v", err)
			}

			var sourceNew, targetNew os.FileInfo
			if sourceNew, err = os.Stat(filepath.Join(stateDir, utils.SOURCE_CONFIG_FILENAME)); err != nil {
				t.Errorf("got unexpected error %#v", err)
			}
			if targetNew, err = os.Stat(filepath.Join(stateDir, utils.TARGET_CONFIG_FILENAME)); err != nil {
				t.Errorf("got unexpected error %#v", err)
			}

			if sourceOld.ModTime() != sourceNew.ModTime() {
				t.Errorf("want %#v got %#v", sourceOld.ModTime(), sourceNew.ModTime())
			}
			if targetOld.ModTime() != targetNew.ModTime() {
				t.Errorf("want %#v got %#v", targetOld.ModTime(), targetNew.ModTime())
			}
		}

		{ // creating cluster config files succeeds on multiple runs
			err = CreateInitialClusterConfigs(oldBinDir, newBinDir)
			if err != nil {
				t.Fatalf("unexpected error %#v", err)
			}
		}
	})
}
