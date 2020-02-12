package hub

import (
	"fmt"
	"os/exec"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/utils"
)

func (s *Server) UpdateConfFiles() error {
	if err := updateGpperfmonConf(s.Target.MasterDataDir()); err != nil {
		return err
	}

	if err := updatePostgresqlConf(s.TargetInitializeConfig.Master.Port, s.Target, s.Source); err != nil {
		return err
	}

	return nil
}

func updateGpperfmonConf(masterDataDir string) error {
	script := fmt.Sprintf(
		"sed 's@log_location = .*$@log_location = %[1]s/gpperfmon/logs@' %[1]s/gpperfmon/conf/gpperfmon.conf > %[1]s/gpperfmon/conf/gpperfmon.conf.updated && "+
			"mv %[1]s/gpperfmon/conf/gpperfmon.conf %[1]s/gpperfmon/conf/gpperfmon.conf.bak && "+
			"mv %[1]s/gpperfmon/conf/gpperfmon.conf.updated %[1]s/gpperfmon/conf/gpperfmon.conf",
		masterDataDir,
	)
	gplog.Debug("executing command: %+v", script) // TODO: Move this debug log into ExecuteLocalCommand()
	cmd := execCommand("bash", "-c", script)
	_, err := cmd.Output()
	if err != nil {
		return xerrors.Errorf("updating %s/gpperfmon/conf/gpperfmon.conf: %w", masterDataDir, err)
	}
	return nil
}

// oldTargetPort is the old port on which the target cluster was initialized.
// This is used to search the postgresql.conf rather than target.MasterPort()
// which has been changed to match the source cluster after updating the
// catalog in a previous substep.
func updatePostgresqlConf(oldTargetPort int, target *utils.Cluster, source *utils.Cluster) error {
	script := fmt.Sprintf(
		"sed 's/port=%d/port=%d/' %[3]s/postgresql.conf > %[3]s/postgresql.conf.updated && "+
			"mv %[3]s/postgresql.conf %[3]s/postgresql.conf.bak && "+
			"mv %[3]s/postgresql.conf.updated %[3]s/postgresql.conf",
		oldTargetPort, source.MasterPort(), target.MasterDataDir(),
	)
	gplog.Debug("executing command: %+v", script) // TODO: Move this debug log into ExecuteLocalCommand()
	cmd := exec.Command("bash", "-c", script)
	_, err := cmd.Output()
	if err != nil {
		return xerrors.Errorf("updating %s/postgresql.conf: %w", target.MasterDataDir(), err)
	}
	return nil
}
