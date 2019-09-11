package commanders

import (
	"context"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/idl"
)

type VersionChecker struct {
	client idl.CliToHubClient
}

func NewVersionChecker(client idl.CliToHubClient) VersionChecker {
	return VersionChecker{
		client: client,
	}
}

func (req VersionChecker) Execute() error {
	resp, err := req.client.CheckVersion(context.Background(), &idl.CheckVersionRequest{})
	if err != nil {
		return errors.Wrap(err, "gRPC call to hub failed")
	}
	if !resp.IsVersionCompatible {
		return errors.New("Version Compatibility Check Failed")
	}

	return nil
}
