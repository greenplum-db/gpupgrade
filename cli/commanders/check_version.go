package commanders

import (
	"context"

	pb "github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

type VersionChecker struct {
	client pb.CliToHubClient
}

func NewVersionChecker(client pb.CliToHubClient) VersionChecker {
	return VersionChecker{
		client: client,
	}
}

func (req VersionChecker) Execute(masterHost string, dbPort int) error {
	resp, err := req.client.CheckVersion(context.Background(),
		&pb.CheckVersionRequest{Host: masterHost, DbPort: int32(dbPort)})
	if err != nil {
		gplog.Error("ERROR - gRPC call to hub failed")
		return err
	}
	if resp.IsVersionCompatible {
		gplog.Info("gpupgrade: Version Compatibility Check [OK]\n")
	} else {
		gplog.Info("gpupgrade: Version Compatibility Check [Failed]\n")
	}
	gplog.Info("Check version request is processed.")

	return nil
}
