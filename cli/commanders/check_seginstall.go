package commanders

import (
	"context"

	pb "github.com/greenplum-db/gpupgrade/idl"
)

type SeginstallChecker struct {
	client pb.CliToHubClient
}

func NewSeginstallCheckerCmd() error {
	client := connectToHub()
	return NewSeginstallChecker(client).Execute()
}

func NewSeginstallChecker(client pb.CliToHubClient) SeginstallChecker {
	return SeginstallChecker{
		client: client,
	}
}

func (req SeginstallChecker) Execute() error {
	_, err := req.client.CheckSeginstall(
		context.Background(),
		&pb.CheckSeginstallRequest{},
	)
	return err
}
