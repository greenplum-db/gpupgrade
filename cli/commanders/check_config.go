package commanders

import (
	"context"

	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

type ConfigChecker struct {
	client idl.CliToHubClient
}

func NewConfigChecker(client idl.CliToHubClient) ConfigChecker {
	return ConfigChecker{
		client: client,
	}
}

func (req ConfigChecker) Execute() error {
	_, err := req.client.CheckConfig(context.Background(),
		&idl.CheckConfigRequest{})
	if err != nil {
		gplog.Error("ERROR - gRPC call to hub failed")
		return err
	}
	gplog.Info("Check config request is processed.")
	return nil
}
