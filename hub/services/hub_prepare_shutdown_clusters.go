package services

import (
	pb "github.com/greenplum-db/gpupgrade/idl"

	"golang.org/x/net/context"

	"path"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

func (h *Hub) PrepareShutdownClusters(ctx context.Context, in *pb.PrepareShutdownClustersRequest) (*pb.PrepareShutdownClustersReply, error) {
	gplog.Info("starting PrepareShutdownClusters()")

	// will be initialized for future uses also? We think so -- it should
	oldPostmasterRunning, newPostmasterRunning := h.clusterPair.EitherPostmasterRunning()
	if oldPostmasterRunning || newPostmasterRunning {
		pathToGpstopStateDir := path.Join(h.conf.StateDir, "gpstop")
		go h.clusterPair.StopEverything(pathToGpstopStateDir, oldPostmasterRunning, newPostmasterRunning)
	} else {
		gplog.Info("PrepareShutdownClusters: neither postmaster was running, nothing to do")
	}

	/* TODO: gpstop may take a while.
	 * How do we check if everything is stopped?
	 * Should we check bindirs for 'good-ness'? No...

	 * Use go routine along with using files as a way to keep track of gpstop state
	 */

	// XXX: May be tell user to run status, or if that seems stuck, check gpAdminLogs/gpupgrade_hub*.log

	return &pb.PrepareShutdownClustersReply{}, nil
}
