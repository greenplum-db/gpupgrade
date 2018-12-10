package commanders

import (
	"context"

	"github.com/greenplum-db/gpupgrade/idl"
)

type SeginstallChecker struct {
	client idl.CliToHubClient
}

func NewSeginstallChecker(client idl.CliToHubClient) SeginstallChecker {
	return SeginstallChecker{
		client: client,
	}
}

func (req SeginstallChecker) Execute() error {
	_, err := req.client.CheckSeginstall(
		context.Background(),
		&idl.CheckSeginstallRequest{},
	)
	return err
}
