package commanders

import (
	"context"

	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

type ObjectCountChecker struct {
	client idl.CliToHubClient
}

func NewObjectCountChecker(client idl.CliToHubClient) ObjectCountChecker {
	return ObjectCountChecker{client: client}
}

func (req ObjectCountChecker) Execute() error {
	reply, err := req.client.CheckObjectCount(context.Background(),
		&idl.CheckObjectCountRequest{})
	if err != nil {
		gplog.Error("ERROR - gRPC call to hub failed")
		return err
	}
	//TODO: do we want to report results to the user earlier? Should we make a gRPC call per db?
	for _, count := range reply.ListOfCounts {
		gplog.Info("Checking object counts in database: %s", count.DbName)
		gplog.Info("Number of AO objects - %d", count.AoCount)
		gplog.Info("Number of heap objects - %d", count.HeapCount)
	}
	gplog.Info("Check object count request is processed.")
	return nil
}
