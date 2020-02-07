package finalize

import (
	"database/sql"
	"fmt"
	"os/exec"

	"github.com/greenplum-db/gpupgrade/hub/cluster"

	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gpupgrade/step"

	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"
)

// ReconfigurePorts executes the tricky sequence of operations required to
// change the ports on a cluster.
//
// TODO: this method needs test coverage.
func ReconfigurePorts(stream step.OutStreams, source *utils.Cluster, target *utils.Cluster) (err error) {
	// 1). bring down the cluster
	err = cluster.StopCluster(stream, target)
	if err != nil {
		return xerrors.Errorf("%s failed to stop cluster: %w",
			idl.Substep_RECONFIGURE_PORTS, err)
	}

	// 2). bring up the master(fts will not "freak out", etc)
	script := fmt.Sprintf("source %s/../greenplum_path.sh && %s/gpstart -am -d %s",
		target.BinDir, target.BinDir, target.MasterDataDir())
	cmd := exec.Command("bash", "-c", script)
	_, err = cmd.Output()
	if err != nil {
		return xerrors.Errorf("%s failed to start target cluster in utility mode: %w",
			idl.Substep_RECONFIGURE_PORTS, err)
	}

	// 3). rewrite gp_segment_configuration with the updated port number
	err = updateSegmentConfiguration(source, target)
	if err != nil {
		return err
	}

	// 4). bring down the master
	script = fmt.Sprintf("source %s/../greenplum_path.sh && %s/gpstop -aim -d %s",
		target.BinDir, target.BinDir, target.MasterDataDir())
	cmd = exec.Command("bash", "-c", script)
	_, err = cmd.Output()
	if err != nil {
		return xerrors.Errorf("%s failed to stop target cluster in utility mode: %w",
			idl.Substep_RECONFIGURE_PORTS, err)
	}

	// 5). rewrite the "port" field in the master's postgresql.conf
	script = fmt.Sprintf(
		"sed 's/port=%d/port=%d/' %[3]s/postgresql.conf > %[3]s/postgresql.conf.updated && "+
			"mv %[3]s/postgresql.conf %[3]s/postgresql.conf.bak && "+
			"mv %[3]s/postgresql.conf.updated %[3]s/postgresql.conf",
		target.MasterPort(), source.MasterPort(), target.MasterDataDir(),
	)
	gplog.Debug("executing command: %+v", script) // TODO: Move this debug log into ExecuteLocalCommand()
	cmd = exec.Command("bash", "-c", script)
	_, err = cmd.Output()
	if err != nil {
		return xerrors.Errorf("%s failed to execute sed command: %w",
			idl.Substep_RECONFIGURE_PORTS, err)
	}

	// 6. bring up the cluster
	script = fmt.Sprintf("source %s/../greenplum_path.sh && %s/gpstart -a -d %s",
		target.BinDir, target.BinDir, target.MasterDataDir())
	cmd = exec.Command("bash", "-c", script)
	_, err = cmd.Output()
	if err != nil {
		return xerrors.Errorf("%s failed to start target cluster: %w",
			idl.Substep_RECONFIGURE_PORTS, err)
	}

	return nil
}

func updateSegmentConfiguration(source, target *utils.Cluster) error {
	connURI := fmt.Sprintf("postgresql://localhost:%d/template1?gp_session_role=utility&allow_system_table_mods=true&search_path=", target.MasterPort())
	targetDB, err := sql.Open("pgx", connURI)
	defer func() {
		closeErr := targetDB.Close()
		if closeErr != nil {
			closeErr = xerrors.Errorf("closing connection to new master db: %w", closeErr)
			err = multierror.Append(err, closeErr)
		}
	}()
	if err != nil {
		return xerrors.Errorf("%s failed to open connection to utility master: %w",
			idl.Substep_RECONFIGURE_PORTS, err)
	}
	err = cluster.ClonePortsFromCluster(targetDB, source.Cluster)
	if err != nil {
		return xerrors.Errorf("%s failed to clone ports: %w",
			idl.Substep_RECONFIGURE_PORTS, err)
	}
	return nil
}
