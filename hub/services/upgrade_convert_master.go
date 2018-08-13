package services

import (
	"fmt"
	"path/filepath"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (h *Hub) UpgradeConvertMaster(ctx context.Context, in *pb.UpgradeConvertMasterRequest) (*pb.UpgradeConvertMasterReply, error) {
	gplog.Info("Starting master upgrade")
	//need to remember where we ran, i.e. pathToUpgradeWD, b/c pg_upgrade generates some files that need to be copied to QE nodes later
	//this is also where the 1.done, 2.inprogress ... files will be written
	err := h.ConvertMaster()
	if err != nil {
		gplog.Error("%v", err)
		return &pb.UpgradeConvertMasterReply{}, err
	}

	return &pb.UpgradeConvertMasterReply{}, nil
}

func (h *Hub) ConvertMaster() error {
	pathToUpgradeWD := filepath.Join(h.conf.StateDir, upgradestatus.CONVERT_MASTER)
	err := utils.System.MkdirAll(pathToUpgradeWD, 0700)
	if err != nil {
		errMsg := fmt.Sprintf("mkdir %s failed: %v. Is there an pg_upgrade in progress?", pathToUpgradeWD, err)
		gplog.Error(errMsg)
		return errors.New(errMsg)
	}

	pgUpgradeLog := filepath.Join(pathToUpgradeWD, "/pg_upgrade_master.log")
	upgradeCmd := fmt.Sprintf("unset PGHOST; unset PGPORT; cd %s && nohup %s "+
		"--old-bindir=%s --old-datadir=%s --old-port=%d "+
		"--new-bindir=%s --new-datadir=%s --new-port=%d "+
		"--dispatcher-mode --progress",
		pathToUpgradeWD,
		filepath.Join(h.target.BinDir, "pg_upgrade"),
		h.source.BinDir,
		h.source.MasterDataDir(),
		h.source.MasterPort(),
		h.target.BinDir,
		h.target.MasterDataDir(),
		h.target.MasterPort())

	gplog.Info("Convert Master upgrade command: %#v", upgradeCmd)

	//export ENV VARS instead of passing on cmd line?
	err = utils.System.RunCommandAsync(upgradeCmd, pgUpgradeLog)
	if err != nil {
		gplog.Error("Error when starting the upgrade: %s", err)
		return err
	}
	gplog.Info("Found no errors when starting the upgrade")
	return nil
}
