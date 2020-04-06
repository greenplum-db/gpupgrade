package agent

import (
	"path/filepath"

	"github.com/greenplum-db/gpupgrade/utils"
)

// IsPostgres returns true if this directory appears to be
//   a postgres data directory.
func IsPostgres(dir string) bool {
	if !utils.DoesPathExist(dir) {
		return false
	}

	for _, fileName := range utils.PostgresFiles {
		filePath := filepath.Join(dir, fileName)
		_, err := utils.System.Stat(filePath)
		if err != nil {
			return false
		}
	}

	return true
}
