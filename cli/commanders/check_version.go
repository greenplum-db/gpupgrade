package commanders

import (
	"context"

	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

type VersionChecker struct {
	client idl.CliToHubClient
}

func NewVersionChecker(client idl.CliToHubClient) VersionChecker {
	return VersionChecker{
		client: client,
	}
}

// TODO: fold into check config or something?
func (req VersionChecker) Execute() error {
	resp, err := req.client.CheckVersion(context.Background(), &idl.CheckVersionRequest{})
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
