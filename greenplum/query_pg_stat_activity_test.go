// Copyright (c) 2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestQueryPgStatActivity(t *testing.T) {
	target := MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "coordinator", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby", Port: 16432, Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Port: 25433, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg1", Port: 25434, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Port: 25435, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg2", Port: 25436, Role: greenplum.MirrorRole},
	})
	target.Destination = idl.ClusterDestination_INTERMEDIATE
	target.Version = semver.MustParse("6.0.0")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("couldn't create sqlmock: %v", err)
	}
	defer testutils.FinishMock(mock, t)

	t.Run("succeeds", func(t *testing.T) {
		expectPgStatActivityToNotReturn(mock)

		err = greenplum.QueryPgStatActivity(db, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("uses correct query for GPDB 5X", func(t *testing.T) {
		target.Version = semver.MustParse("5.0.0")
		defer func() {
			target.Version = semver.MustParse("6.0.0")
		}()

		mock.ExpectQuery(`SELECT datname, usename, application_name, current_query FROM pg_stat_activity WHERE procpid <> pg_backend_pid\(\);`).
			WillReturnRows(sqlmock.NewRows([]string{"datname", "usename", "application_name", "query"}))

		err = greenplum.QueryPgStatActivity(db, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("errors when pg_stat_activity shows active connections", func(t *testing.T) {
		expectPgStatActivityToReturn(mock).WillReturnRows(sqlmock.NewRows([]string{"datname", "usename", "application_name", "query"}).
			AddRow("postgres", "gpadmin", "etl_job", "SELECT * FROM my_table;").
			AddRow("stats_db", "gpcc", "status_checker", "SELECT * FROM stats;"))

		expected := greenplum.StatActivities{
			{Usename: "gpadmin", Application_name: "etl_job", Datname: "postgres", Query: "SELECT * FROM my_table;"},
			{Usename: "gpcc", Application_name: "status_checker", Datname: "stats_db", Query: "SELECT * FROM stats;"},
		}

		err = greenplum.QueryPgStatActivity(db, target)
		var nextActionsErr utils.NextActionErr
		if !errors.As(err, &nextActionsErr) {
			t.Errorf("got type %T want %T", err, nextActionsErr)
		}

		if !strings.Contains(nextActionsErr.Err.Error(), expected.Error()) {
			t.Errorf("got %#v, want %#v", err, expected)
		}

		if !strings.Contains(nextActionsErr.NextAction, "close") {
			t.Errorf("got %q, want 'close'", nextActionsErr.NextAction)
		}
	})

	t.Run("errors when failing to query", func(t *testing.T) {
		expected := os.ErrPermission
		expectPgStatActivityToReturn(mock).WillReturnError(expected)

		err = greenplum.QueryPgStatActivity(db, target)
		if !errors.Is(err, expected) {
			t.Errorf("got %v want %v", err, expected)
		}
	})

	t.Run("errors when failing to scan", func(t *testing.T) {
		expectPgStatActivityToReturn(mock).WillReturnRows(sqlmock.NewRows([]string{"datname", "usename"}).
			AddRow("postgres", "gpadmin")) // return less fields than scan expects

		err = greenplum.QueryPgStatActivity(db, target)
		if !strings.Contains(err.Error(), "Scan") {
			t.Errorf(`expected %v to contain "Scan"`, err)
		}
	})

	t.Run("errors when iterating the rows cals", func(t *testing.T) {
		expected := os.ErrPermission
		expectPgStatActivityToReturn(mock).WillReturnRows(sqlmock.NewRows([]string{"datname", "usename", "application_name", "query"}).
			AddRow("postgres", "gpadmin", "etl_job", "SELECT * FROM my_table;").
			RowError(0, expected))

		err = greenplum.QueryPgStatActivity(db, target)
		if !errors.Is(err, expected) {
			t.Errorf("got %v want %v", err, expected)
		}
	})
}

func expectPgStatActivityToNotReturn(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT datname, usename, application_name, query FROM pg_stat_activity WHERE pid <> pg_backend_pid\(\);`).
		WillReturnRows(sqlmock.NewRows([]string{"datname", "usename", "application_name", "query"}))
}

func expectPgStatActivityToReturn(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(`SELECT datname, usename, application_name, query FROM pg_stat_activity WHERE pid <> pg_backend_pid\(\);`)
}
