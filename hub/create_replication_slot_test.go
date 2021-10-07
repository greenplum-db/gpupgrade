//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/testutils"
)

func TestCreateReplicationSlots(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("couldn't create sqlmock: %v", err)
	}
	defer testutils.FinishMock(mock, t)

	t.Run("creates replication slots when there are none", func(t *testing.T) {
		mock.ExpectQuery(`SELECT COUNT\(slot_name\) FROM gp_dist_random\('pg_replication_slots'\) WHERE slot_name = 'internal_wal_replication_slot';`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		mock.ExpectQuery(`SELECT pg_create_physical_replication_slot\('internal_wal_replication_slot'\) from gp_dist_random\('gp_id'\);`).
			WillReturnRows(sqlmock.NewRows([]string{"pg_create_physical_replication_slot"}).
				AddRow("(internal_wal_replication_slot,)").
				AddRow("(internal_wal_replication_slot,)").
				AddRow("(internal_wal_replication_slot,)"))

		err = hub.CreateReplicationSlots(db)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("deletes existing replication slots for idempotence", func(t *testing.T) {
		mock.ExpectQuery(`SELECT COUNT\(slot_name\) FROM gp_dist_random\('pg_replication_slots'\) WHERE slot_name = 'internal_wal_replication_slot';`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

		mock.ExpectQuery(`SELECT pg_drop_replication_slot\('internal_wal_replication_slot'\) from gp_dist_random\('gp_id'\);`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

		mock.ExpectQuery(`SELECT pg_create_physical_replication_slot\('internal_wal_replication_slot'\) from gp_dist_random\('gp_id'\);`).
			WillReturnRows(sqlmock.NewRows([]string{"pg_create_physical_replication_slot"}).
				AddRow("(internal_wal_replication_slot,)").
				AddRow("(internal_wal_replication_slot,)").
				AddRow("(internal_wal_replication_slot,)"))

		err = hub.CreateReplicationSlots(db)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("errors when querying pg_replication_slots fails", func(t *testing.T) {
		expected := errors.New("connection failed")

		mock.ExpectQuery(`SELECT COUNT\(slot_name\) FROM gp_dist_random\('pg_replication_slots'\) WHERE slot_name = 'internal_wal_replication_slot';`).
			WillReturnError(expected)

		err = hub.CreateReplicationSlots(db)
		if !errors.Is(err, expected) {
			t.Errorf("got %#v want %#v", err, expected)
		}
	})

	t.Run("errors when pg_drop_replication_slot fails", func(t *testing.T) {
		expected := errors.New("connection failed")

		mock.ExpectQuery(`SELECT COUNT\(slot_name\) FROM gp_dist_random\('pg_replication_slots'\) WHERE slot_name = 'internal_wal_replication_slot';`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

		mock.ExpectQuery(`SELECT pg_drop_replication_slot\('internal_wal_replication_slot'\) from gp_dist_random\('gp_id'\);`).
			WillReturnError(expected)

		err = hub.CreateReplicationSlots(db)
		if !errors.Is(err, expected) {
			t.Errorf("got %#v want %#v", err, expected)
		}
	})

	t.Run("errors when pg_create_physical_replication_slot fails", func(t *testing.T) {
		expected := errors.New("connection failed")

		mock.ExpectQuery(`SELECT COUNT\(slot_name\) FROM gp_dist_random\('pg_replication_slots'\) WHERE slot_name = 'internal_wal_replication_slot';`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		mock.ExpectQuery(`SELECT pg_create_physical_replication_slot\('internal_wal_replication_slot'\) from gp_dist_random\('gp_id'\);`).
			WillReturnError(expected)

		err = hub.CreateReplicationSlots(db)
		if !errors.Is(err, expected) {
			t.Errorf("got %#v want %#v", err, expected)
		}
	})
}
