// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package testutils

import (
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/greenplum-db/gpupgrade/greenplum"
)

// finishMock is a defer function to make the sqlmock API a little bit more like
// gomock. Use it like this:
//
//     db, mock, err := sqlmock.New()
//     if err != nil {
//         t.Fatalf("couldn't create sqlmock: %v", err)
//     }
//     defer finishMock(mock, t)
//
func FinishMock(mock sqlmock.Sqlmock, t *testing.T) {
	t.Helper()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("%v", err)
	}
}

// MockSegmentConfiguration returns a set of sqlmock.Rows that contains the
// expected response to a gp_segment_configuration query.
//
// When changing this implementation, make sure you change MockCluster() to
// match!
func MockSegmentConfiguration() *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"dbid", "contentid", "port", "hostname", "datadir", "role"})
	rows.AddRow(1, -1, 15432, "mdw", "/data/coordinator/gpseg-1", "p")
	rows.AddRow(2, 0, 25432, "sdw1", "/data/primary/gpseg0", "p")

	return rows
}

// MockCluster returns the Cluster equivalent of MockSegmentConfiguration().
//
// When changing this implementation, make sure you change
// MockSegmentConfiguration() to match!
func MockCluster() *greenplum.Cluster {
	segments := greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Port: 15432, Hostname: "mdw", DataDir: "/data/coordinator/gpseg-1", Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: 0, Port: 25432, Hostname: "sdw1", DataDir: "/data/primary/gpseg0", Role: greenplum.PrimaryRole},
	}

	cluster, err := greenplum.NewCluster(segments)
	if err != nil {
		panic(err)
	}

	return &cluster
}
