package commanders

import (
	"context"
	"fmt"

	"github.com/greenplum-db/gpupgrade/idl"
)

type Preparer struct {
	client idl.CliToHubClient
}

func NewPreparer(client idl.CliToHubClient) Preparer {
	return Preparer{client: client}
}

func (p Preparer) ShutdownClusters() error {
	_, err := p.client.PrepareShutdownClusters(context.Background(),
		&idl.PrepareShutdownClustersRequest{})
	if err != nil {
		return err
	}

	fmt.Println("clusters shut down successfully")
	return nil
}

func (p Preparer) InitCluster() error {
	_, err := p.client.PrepareInitCluster(context.Background(), &idl.PrepareInitClusterRequest{})
	if err != nil {
		return err
	}

	fmt.Println("cluster successfully initialized")
	return nil
}
