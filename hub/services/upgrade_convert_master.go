package services

import (
	"fmt"
	"path/filepath"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/log"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (h *Hub) UpgradeConvertMaster(ctx context.Context, in *pb.UpgradeConvertMasterRequest) (*pb.UpgradeConvertMasterReply, error) {
	step := h.checklist.GetStepWriter(upgradestatus.CONVERT_MASTER)
	err := step.ResetStateDir()
	if err != nil {
		return nil, errors.Wrap(err, "could not reset state dir")
	}

	err = step.MarkInProgress()
	if err != nil {
		return nil, errors.Wrap(err, "could not mark in progress")
	}

	go func() {
		defer log.WritePanics()

		err := h.ConvertMaster()
		if err != nil {
			gplog.Error(err.Error())
			step.MarkFailed()
		} else {
			step.MarkComplete()
		}
	}()

	return &pb.UpgradeConvertMasterReply{}, nil
}

func (h *Hub) ConvertMaster() error {
	gplog.Info("Starting master upgrade")

	pathToUpgradeWD := utils.MasterPGUpgradeDirectory(h.conf.StateDir)
	err := utils.System.MkdirAll(pathToUpgradeWD, 0700)
	if err != nil {
		return errors.Wrapf(err, "mkdir %s failed", pathToUpgradeWD)
	}

	pgUpgradeCmd := fmt.Sprintf("unset PGHOST; unset PGPORT; %s "+
		"--old-bindir=%s --old-datadir=%s --old-port=%d "+
		"--new-bindir=%s --new-datadir=%s --new-port=%d "+
		"--mode=dispatcher",
		filepath.Join(h.target.BinDir, "pg_upgrade"),
		h.source.BinDir,
		h.source.MasterDataDir(),
		h.source.MasterPort(),
		h.target.BinDir,
		h.target.MasterDataDir(),
		h.target.MasterPort())

	gplog.Info("Convert Master upgrade command: %#v", pgUpgradeCmd)

	output, err := h.source.Executor.ExecuteLocalCommand(pgUpgradeCmd)
	if err != nil {
		gplog.Error("pg_upgrade failed to start: %s", output)
		return errors.Wrapf(err, "pg_upgrade on master segment failed")
	}

	return nil
}
