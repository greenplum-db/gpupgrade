package services

import (
	"fmt"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/log"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (h *Hub) UpgradeConvertMaster(ctx context.Context, in *idl.UpgradeConvertMasterRequest) (*idl.UpgradeConvertMasterReply, error) {
	gplog.Info("starting %s", upgradestatus.CONVERT_MASTER)

	step, err := h.InitializeStep(upgradestatus.CONVERT_MASTER)
	if err != nil {
		gplog.Error(err.Error())
		return &idl.UpgradeConvertMasterReply{}, err
	}

	go func() {
		defer log.WritePanics()

		if err := h.ConvertMaster(); err != nil {
			gplog.Error(err.Error())
			step.MarkFailed()
		} else {
			step.MarkComplete()
		}
	}()

	return &idl.UpgradeConvertMasterReply{}, nil
}

func (h *Hub) ConvertMaster() error {
	pathToUpgradeWD := utils.MasterPGUpgradeDirectory(h.conf.StateDir)
	err := utils.System.MkdirAll(pathToUpgradeWD, 0700)
	if err != nil {
		return errors.Wrapf(err, "mkdir %s failed", pathToUpgradeWD)
	}

	// pg_upgrade changed its API in GPDB 6.0.
	var modeStr string
	if h.target.Version.Before("6.0.0") {
		modeStr = "--dispatcher-mode"
	} else {
		modeStr = "--mode=dispatcher"
	}

	pgUpgradeCmd := fmt.Sprintf("source %s; cd %s; unset PGHOST; unset PGPORT; "+
		"%s --old-bindir=%s --old-datadir=%s --old-port=%d "+
		"--new-bindir=%s --new-datadir=%s --new-port=%d %s",
		filepath.Join(h.target.BinDir, "..", "greenplum_path.sh"),
		pathToUpgradeWD,
		filepath.Join(h.target.BinDir, "pg_upgrade"),
		h.source.BinDir,
		h.source.MasterDataDir(),
		h.source.MasterPort(),
		h.target.BinDir,
		h.target.MasterDataDir(),
		h.target.MasterPort(),
		modeStr)

	gplog.Info("Convert Master upgrade command: %#v", pgUpgradeCmd)

	output, err := h.source.Executor.ExecuteLocalCommand(pgUpgradeCmd)
	if err != nil {
		gplog.Error("pg_upgrade failed to start: %s", output)
		return errors.Wrapf(err, "pg_upgrade on master segment failed")
	}

	return nil
}
