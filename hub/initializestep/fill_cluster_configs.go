package initializestep

import (
	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/db"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

type saveConfigFunc func(source *utils.Cluster, target *utils.Cluster) error

// create old/new clusters, write to disk and re-read from disk to make sure it is "durable"
func FillClusterConfigsSubStep(_ step.OutStreams, request *idl.InitializeRequest, saveConfig saveConfigFunc) (source *utils.Cluster, target *utils.Cluster, err error) {
	conn := db.NewDBConn("localhost", int(request.SourcePort), "template1")
	defer conn.Close()

	source, err = utils.ClusterFromDB(conn, request.SourceBinDir)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not retrieve source configuration")
	}

	target = &utils.Cluster{Cluster: new(cluster.Cluster), BinDir: request.TargetBinDir}

	if err := saveConfig(source, target); err != nil {
		return source, target, err
	}

	return source, target, nil
}
