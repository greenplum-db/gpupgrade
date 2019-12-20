package hub

import (
	"os"
	"path/filepath"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (h *Hub) Execute(request *idl.ExecuteRequest, stream idl.CliToHub_ExecuteServer) (err error) {
	// Create a log file to contain execute output.
	log, err := utils.System.OpenFile(
		filepath.Join(utils.GetStateDir(), "execute.log"),
		os.O_WRONLY|os.O_CREATE,
		0600,
	)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := log.Close(); closeErr != nil {
			err = multierror.Append(err,
				xerrors.Errorf("failed to close execute log: %w", closeErr))
		}
	}()

	executeStream := newMultiplexedStream(stream, log)

	_, err = log.WriteString("\nExecute in progress.\n")
	if err != nil {
		return xerrors.Errorf("failed writing to execute log: %w", err)
	}

	err = h.Substep(executeStream, upgradestatus.UPGRADE_MASTER,
		func(streams OutStreams) error {
			stateDir := h.conf.StateDir
			return UpgradeMaster(h.source, h.target, stateDir, streams, false)
		})
	if err != nil {
		return err
	}

	err = h.Substep(executeStream, upgradestatus.COPY_MASTER, h.CopyMasterDataDir)
	if err != nil {
		return err
	}

	err = h.Substep(executeStream, upgradestatus.UPGRADE_PRIMARIES,
		func(_ OutStreams) error {
			return h.ConvertPrimaries(false)
		})
	if err != nil {
		return err
	}

	err = h.Substep(executeStream, upgradestatus.START_TARGET_CLUSTER,
		func(streams OutStreams) error {
			return StartCluster(streams, h.target)
		})
	return err
}
