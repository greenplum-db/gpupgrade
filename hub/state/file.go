package state

import (
	"path/filepath"
)

const ConfigFileName = "config.json"

// TODO: make private to package
func GetConfigFilepath(stateDir string) string {
	return filepath.Join(stateDir, ConfigFileName)
}
