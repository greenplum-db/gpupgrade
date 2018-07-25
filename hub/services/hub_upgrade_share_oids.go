package services

import (
	"path/filepath"
	"strings"

	pb "github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/net/context"
)

func (h *Hub) UpgradeShareOids(ctx context.Context, in *pb.UpgradeShareOidsRequest) (*pb.UpgradeShareOidsReply, error) {
	gplog.Info("Started processing share-oids request")

	go h.shareOidFiles()

	return &pb.UpgradeShareOidsReply{}, nil
}

func (h *Hub) shareOidFiles() {
	step := h.checklist.GetStepWriter(upgradestatus.SHARE_OIDS)

	err := step.ResetStateDir()
	if err != nil {
		gplog.Error("error from ResetStateDir " + err.Error())
		return
	}
	err = step.MarkInProgress()
	if err != nil {
		gplog.Error("error from MarkInProgress " + err.Error())
		return
	}

	hostnames := h.source.GetHostnames()

	user := "gpadmin"
	rsyncFlags := "-rzpogt"
	sourceDir := filepath.Join(h.conf.StateDir, "pg_upgrade")

	anyFailed := false
	for _, host := range hostnames {
		destinationDirectory := user + "@" + host + ":" + filepath.Join(h.conf.StateDir, "pg_upgrade")

		rsyncCommand := strings.Join([]string{"rsync", rsyncFlags, filepath.Join(sourceDir, "pg_upgrade_dump_*_oids.sql"), destinationDirectory}, " ")
		gplog.Info("share oids command: %+v", rsyncCommand)

		output, err := h.source.Executor.ExecuteLocalCommand(rsyncCommand)
		if err != nil {
			gplog.Error("share oids failed %s: %s", output, err)
			anyFailed = true
		}
	}
	if anyFailed {
		step.MarkFailed()
		if err != nil {
			gplog.Error("error from MarkFailed " + err.Error())
		}
	} else {
		step.MarkComplete()
		if err != nil {
			gplog.Error("error from MarkComplete " + err.Error())
		}
	}

}
