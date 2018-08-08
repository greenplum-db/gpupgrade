package services

import (
	"os"
	"strconv"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"golang.org/x/net/context"
)

func (h *Hub) CheckConfig(ctx context.Context, _ *pb.CheckConfigRequest) (*pb.CheckConfigReply, error) {
	gplog.Info("starting CheckConfig()")

	c := upgradestatus.NewChecklistManager(h.conf.StateDir)
	step := c.GetStepWriter(upgradestatus.CONFIG)
	// TODO: We do this here and in init-cluster; we should probably do it everywhere
	initializeState(step)

	port, err := strconv.Atoi(os.Getenv("PGPORT"))
	if err != nil {
		port = 5432 // follow postgres convention for default port
	}

	h.source = utils.NewMasterOnlyCluster(port, "localhost", h.source.BinDir, h.source.ConfigPath)
	dbConnector := h.source.NewDBConn()
	err = h.source.RefreshConfig(dbConnector)
	if err != nil {
		step.MarkFailed()
		gplog.Error(err.Error())
		return &pb.CheckConfigReply{}, err
	}
	err = h.source.Commit()
	if err != nil {
		step.MarkFailed()
		gplog.Error(err.Error())
		return &pb.CheckConfigReply{}, err
	}

	successReply := &pb.CheckConfigReply{ConfigStatus: "All good"}
	step.MarkComplete()

	return successReply, nil
}

func initializeState(step upgradestatus.StateWriter) {
	err := step.ResetStateDir()
	if err != nil {
		gplog.Fatal(err, "Could not reset step directory for %s", step)
	}

	err = step.MarkInProgress()
	if err != nil {
		gplog.Fatal(err, "Could not mark step %s in progress", step)
	}
}
