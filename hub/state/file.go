package state

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/utils"
)

const ConfigFileName = "config.json"

func GetConfigFilepath(stateDir string) string {
	return filepath.Join(stateDir, ConfigFileName)
}

func CreateConfigFile() error {
	// if empty json configuration file exists, skip recreating it
	filepath := GetConfigFilepath(utils.GetStateDir())
	_, err := os.Stat(filepath)

	// if the file exists, there will be no error or if there is an error it might
	// also indicate that the file exists, in either case don't overwrite the file
	if err == nil || os.IsExist(err) {
		gplog.Debug("Initial cluster configuration file %s already present...skipping", filepath)
		return nil
	}

	// if the err is anything other than file does not exist, error out
	if !os.IsNotExist(err) {
		gplog.Debug("Check to find presence of initial cluster configuration file %s failed", filepath)
		return err
	}

	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	fmt.Fprintf(file, "{}") // the hub will fill this in during initialization

	return nil
}
