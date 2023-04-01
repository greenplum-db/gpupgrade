// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package connection

import (
	"database/sql"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
)

// Bootstrap returns a sql.DB connection. Most callers will use the Connection
// function on the cluster object. However, Bootstrap is useful for when a
// cluster object does not exist and a database connection is needed.
func Bootstrap(destination idl.ClusterDestination, gphome string, port int) (*sql.DB, error) {
	cluster, err := greenplum.NewCluster([]greenplum.SegConfig{})
	if err != nil {
		return nil, err
	}

	// destination and version are needed when creating the connection
	cluster.Destination = destination
	cluster.Version, err = greenplum.Version(gphome)
	if err != nil {
		return nil, err
	}

	conn := cluster.Connection([]greenplum.Option{greenplum.Port(port)}...)
	db, err := sql.Open("pgx", conn)
	if err != nil {
		return nil, err
	}

	return db, nil
}
