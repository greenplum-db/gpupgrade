package commanders

import (
	"context"
	pb "github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

type DiskSpaceChecker struct {
	client pb.CliToHubClient
}

func NewDiskSpaceChecker(client pb.CliToHubClient) DiskSpaceChecker {
	return DiskSpaceChecker{client: client}
}

func (req DiskSpaceChecker) Execute() error {
	reply, err := req.client.CheckDiskSpace(context.Background(),
		&pb.CheckDiskSpaceRequest{})
	if err != nil {
		gplog.Error("ERROR - gRPC call to hub failed")
		return err
	}

	//TODO: do we want to report results to the user earlier? Should we make a gRPC call per db?
	for _, segmentFileSysUsage := range reply.SegmentFileSysUsage {
		gplog.Info(segmentFileSysUsage)
	}
	gplog.Info("Check disk space request is processed.")
	return nil
}
