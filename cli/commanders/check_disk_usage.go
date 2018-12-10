package commanders

import (
	"context"

	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

type DiskSpaceChecker struct {
	client idl.CliToHubClient
}

func NewDiskSpaceChecker(client idl.CliToHubClient) DiskSpaceChecker {
	return DiskSpaceChecker{client: client}
}

func (req DiskSpaceChecker) Execute() error {
	reply, err := req.client.CheckDiskSpace(context.Background(),
		&idl.CheckDiskSpaceRequest{})
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
