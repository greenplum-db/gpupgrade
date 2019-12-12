package services

import (
	"database/sql"
	"fmt"
	"os/exec"

	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	multierror "github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
)

func (h *Hub) UpgradeReconfigurePortsSubStep(stream *multiplexedStream) error {
	gplog.Info("starting %s", upgradestatus.RECONFIGURE_PORTS)

	step, err := h.InitializeStep(upgradestatus.RECONFIGURE_PORTS, stream.stream)
	if err != nil {
		gplog.Error(err.Error())
		return err
	}

	if err := h.reconfigurePorts(stream); err != nil {
		gplog.Error(err.Error())

		// Log any stderr from failed commands.
		var exitErr *exec.ExitError
		if xerrors.As(err, &exitErr) {
			gplog.Debug(string(exitErr.Stderr))
		}

		step.MarkFailed()
		return err
	}

	step.MarkComplete()
	return nil
}

// reconfigurePorts executes the tricky sequence of operations required to
// change the ports on a cluster.
//
// TODO: this method needs test coverage.
func (h *Hub) reconfigurePorts(stream *multiplexedStream) (err error) {
	// 1). bring down the cluster
	err = StopCluster(stream, h.target)
	if err != nil {
		return xerrors.Errorf("%s failed to stop cluster: %w",
			upgradestatus.RECONFIGURE_PORTS, err)
	}

	// 2). bring up the master(fts will not "freak out", etc)
	script := fmt.Sprintf("source %s/../greenplum_path.sh && %s/gpstart -am -d %s",
		h.target.BinDir, h.target.BinDir, h.target.MasterDataDir())
	cmd := exec.Command("bash", "-c", script)
	_, err = cmd.Output()
	if err != nil {
		return xerrors.Errorf("%s failed to start target cluster in utility mode: %w",
			upgradestatus.RECONFIGURE_PORTS, err)
	}

	// 3). rewrite gp_segment_configuration with the updated port number
	err = updateSegmentConfiguration(h.source, h.target)
	if err != nil {
		return err
	}

	// 4). bring down the master
	script = fmt.Sprintf("source %s/../greenplum_path.sh && %s/gpstop -aim -d %s",
		h.target.BinDir, h.target.BinDir, h.target.MasterDataDir())
	cmd = exec.Command("bash", "-c", script)
	_, err = cmd.Output()
	if err != nil {
		return xerrors.Errorf("%s failed to stop target cluster in utility mode: %w",
			upgradestatus.RECONFIGURE_PORTS, err)
	}

	// 5). rewrite the "port" field in the master's postgresql.conf
	script = fmt.Sprintf(
		"sed 's/port=%d/port=%d/' %[3]s/postgresql.conf > %[3]s/postgresql.conf.updated && "+
			"mv %[3]s/postgresql.conf %[3]s/postgresql.conf.bak && "+
			"mv %[3]s/postgresql.conf.updated %[3]s/postgresql.conf",
		h.target.MasterPort(), h.source.MasterPort(), h.target.MasterDataDir(),
	)
	gplog.Debug("executing command: %+v", script) // TODO: Move this debug log into ExecuteLocalCommand()
	cmd = exec.Command("bash", "-c", script)
	_, err = cmd.Output()
	if err != nil {
		return xerrors.Errorf("%s failed to execute sed command: %w",
			upgradestatus.RECONFIGURE_PORTS, err)
	}

	// 6. bring up the cluster
	script = fmt.Sprintf("source %s/../greenplum_path.sh && %s/gpstart -a -d %s",
		h.target.BinDir, h.target.BinDir, h.target.MasterDataDir())
	cmd = exec.Command("bash", "-c", script)
	_, err = cmd.Output()
	if err != nil {
		return xerrors.Errorf("%s failed to start target cluster: %w",
			upgradestatus.RECONFIGURE_PORTS, err)
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
			upgradestatus.RECONFIGURE_PORTS, err)
	}
	err = ClonePortsFromCluster(targetDB, source.Cluster)
	if err != nil {
		return xerrors.Errorf("%s failed to clone ports: %w",
			upgradestatus.RECONFIGURE_PORTS, err)
	}
	return nil
}
