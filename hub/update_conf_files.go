package hub

import (
	"fmt"

	"github.com/greenplum-db/gpupgrade/utils"
)

func (s *Server) UpdateConfFiles() error {
	if err := UpdateGpperfmonConf(s.Target.MasterDataDir()); err != nil {
		return err
	}

	if err := UpdatePostgresqlConf(s.TargetInitializeConfig.Master.Port, s.Target, s.Source); err != nil {
		return err
	}

	return nil
}

func UpdateGpperfmonConf(masterDataDir string) error {
	configFile := fmt.Sprintf("%[1]s/gpperfmon/conf/gpperfmon.conf", masterDataDir)
	replacement := fmt.Sprintf("log_location = %[1]s/gpperfmon/logs", masterDataDir)

	return ReplaceStringWithinFile("log_location = .*$",
		replacement,
		configFile)
}

// oldTargetPort is the old port on which the target cluster was initialized.
// This is used to search the postgresql.conf rather than target.MasterPort()
// which has been changed to match the source cluster after updating the
// catalog in a previous substep.
func UpdatePostgresqlConf(oldTargetPort int, target *utils.Cluster, source *utils.Cluster) error {
	return ReplaceStringWithinFile(
		fmt.Sprintf("port=%d", oldTargetPort),
		fmt.Sprintf("port=%d", source.MasterPort()),
		fmt.Sprintf("%[1]s/postgresql.conf", target.MasterDataDir()),
	)
}
