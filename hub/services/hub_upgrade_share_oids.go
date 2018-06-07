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
	c := upgradestatus.NewChecklistManager(h.conf.StateDir)
	shareOidsStep := "share-oids"

	err := c.ResetStateDir(shareOidsStep)
	if err != nil {
		gplog.Error("error from ResetStateDir " + err.Error())
	}
	err = c.MarkInProgress(shareOidsStep)
	if err != nil {
		gplog.Error("error from MarkInProgress " + err.Error())
	}

	hostnames := h.clusterPair.GetHostnames()

	user := "gpadmin"
	rsyncFlags := "-rzpogt"
	sourceDir := filepath.Join(h.conf.StateDir, "pg_upgrade")

	anyFailed := false
	for _, host := range hostnames {
		destinationDirectory := user + "@" + host + ":" + filepath.Join(h.conf.StateDir, "pg_upgrade")

		rsyncArgs := strings.Join([]string{"rsync", rsyncFlags, filepath.Join(sourceDir, "pg_upgrade_dump_*_oids.sql"), destinationDirectory}, " ")
		rsyncCommand := h.commandExecer("bash", "-c", rsyncArgs)
		gplog.Info("share oids command: %+v", rsyncCommand)

		output, err := rsyncCommand.CombinedOutput()
		if err != nil {
			var out string
			if len(output) != 0 {
				out = string(output)
			}
			gplog.Error("share oids failed %s: %s", out, err)

			c.MarkFailed(shareOidsStep)
			anyFailed = true
		}
	}
	if !anyFailed {
		c.MarkComplete(shareOidsStep)
	}
}
