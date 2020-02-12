package hub

import (
	"fmt"
	"os/exec"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	multierror "github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func UpdateMasterConf(source, target *utils.Cluster) error {
	var multiErr *multierror.Error

	multierror.Append(multiErr,
		updateGpperfmonConf(source.MasterDataDir()))

	multierror.Append(multiErr,
		updatePostgresConfig(target, source))

	return multiErr.ErrorOrNil()
}

func updateGpperfmonConf(newDataDir string) error {
	script := fmt.Sprintf(
		"sed 's@log_location = .*$@log_location = %[1]s/gpperfmon/logs@' %[1]s/gpperfmon/conf/gpperfmon.conf > %[1]s/gpperfmon/conf/gpperfmon.conf.updated && "+
			"mv %[1]s/gpperfmon/conf/gpperfmon.conf %[1]s/gpperfmon/conf/gpperfmon.conf.bak && "+
			"mv %[1]s/gpperfmon/conf/gpperfmon.conf.updated %[1]s/gpperfmon/conf/gpperfmon.conf",
		newDataDir,
	)
	gplog.Debug("executing command: %+v", script) // TODO: Move this debug log into ExecuteLocalCommand()
	cmd := execCommand("bash", "-c", script)
	_, err := cmd.Output()
	if err != nil {
		return xerrors.Errorf("%s failed to execute sed command: %w",
			idl.Substep_FINALIZE_UPDATE_POSTGRESQL_CONF, err)
	}
	return nil
}

func updatePostgresConfig(target *utils.Cluster, source *utils.Cluster) error {
	script := fmt.Sprintf(
		"sed 's/port=%d/port=%d/' %[3]s/postgresql.conf > %[3]s/postgresql.conf.updated && "+
			"mv %[3]s/postgresql.conf %[3]s/postgresql.conf.bak && "+
			"mv %[3]s/postgresql.conf.updated %[3]s/postgresql.conf",
		target.MasterPort(), source.MasterPort(), target.MasterDataDir(),
	)
	gplog.Debug("executing command: %+v", script) // TODO: Move this debug log into ExecuteLocalCommand()
	cmd := exec.Command("bash", "-c", script)
	_, err := cmd.Output()
	if err != nil {
		return xerrors.Errorf("%s failed to execute sed command: %w",
			idl.Substep_FINALIZE_UPDATE_POSTGRESQL_CONF, err)
	}
	return nil
}
