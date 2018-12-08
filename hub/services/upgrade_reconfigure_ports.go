package services

import (
	"context"
	"fmt"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"github.com/greenplum-db/gpupgrade/utils/log"
)

const (
	SedAndMvString = "sed 's/port=%d/port=%d/' %[3]s/postgresql.conf > %[3]s/postgresql.conf.updated && " +
		"mv %[3]s/postgresql.conf %[3]s/postgresql.conf.bak && " +
		"mv %[3]s/postgresql.conf.updated %[3]s/postgresql.conf"
)

func (h *Hub) UpgradeReconfigurePorts(ctx context.Context, in *idl.UpgradeReconfigurePortsRequest) (*idl.UpgradeReconfigurePortsReply, error) {
	gplog.Info("starting %s", upgradestatus.RECONFIGURE_PORTS)
	defer log.WritePanics()

	stepWriter, err := h.WriteStep(upgradestatus.RECONFIGURE_PORTS)
	if err != nil {
		gplog.Error(err.Error())
		return &idl.UpgradeReconfigurePortsReply{}, err
	}

	err = h.reconfigurePorts()
	if err != nil {
		gplog.Error(err.Error())
		stepWriter.MarkFailed()
		return &idl.UpgradeReconfigurePortsReply{}, err
	}

	stepWriter.MarkComplete()
	return &idl.UpgradeReconfigurePortsReply{}, nil
}

func (h *Hub) reconfigurePorts() error {
	sedCommand := fmt.Sprintf(SedAndMvString, h.target.MasterPort(), h.source.MasterPort(), h.target.MasterDataDir())
	gplog.Debug("executing reconfigure-ports sed command: %s", sedCommand)

	output, err := h.source.Executor.ExecuteLocalCommand(sedCommand)
	if err != nil {
		return errors.Wrapf(err, "reconfigure-ports sed command failed with: %s", output)
	}

	return nil
}